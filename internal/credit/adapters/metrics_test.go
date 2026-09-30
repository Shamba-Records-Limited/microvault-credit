package adapters

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
)

type metricsLoanRepo struct {
	repository.LoanRepository
	loan    *models.Loan
	updates int
}

func (r *metricsLoanRepo) GetBySequenceID(context.Context, string) (*models.Loan, error) {
	cp := *r.loan
	return &cp, nil
}

func (r *metricsLoanRepo) GetByID(context.Context, string) (*models.Loan, error) {
	cp := *r.loan
	return &cp, nil
}

func (r *metricsLoanRepo) Update(_ context.Context, l *models.Loan) error {
	r.updates++
	*r.loan = *l
	return nil
}

func counterValue(t *testing.T, reader *sdkmetric.ManualReader, name string, attrs ...attribute.KeyValue) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	want := attribute.NewSet(attrs...)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
				if dp.Attributes.Equals(&want) {
					return dp.Value
				}
			}
		}
	}
	return 0
}

func TestBusinessCountersCountTransitionsOnce(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	provider := "moneygram"
	repo := &metricsLoanRepo{loan: &models.Loan{ID: "loan-1", Status: models.LoanStatusDisbursing, RampProvider: &provider}}
	disb := NewDisbursementStatusAdapter(DisbursementAdapterDeps{Repo: repo, Logger: slog.New(slog.DiscardHandler)})

	ctx := t.Context()
	require.NoError(t, disb.UpdateDisbursementStatus(ctx, "seq-1", models.DisbursementStatusCompleted))
	require.NoError(t, disb.UpdateDisbursementStatus(ctx, "seq-1", models.DisbursementStatusCompleted))
	require.Equal(t, int64(1), counterValue(t, reader, "microvault.loan.disbursements",
		attribute.String("provider", "moneygram"), attribute.String("outcome", models.DisbursementStatusCompleted)),
		"a replayed terminal status must not count twice")

	dep, err := NewMoneyGramDepositAdapter(DepositAdapterDeps{
		Repo:       &metricsLoanRepo{loan: &models.Loan{ID: "loan-2", RepaymentStatus: models.LoanRepaymentStatusInitiated}},
		LoanSvc:    &fakeLoanSvc{},
		StellarSvc: stubStellarService{},
		Logger:     slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)
	require.NoError(t, dep.MarkExpired(ctx, "loan-2"))
	require.Equal(t, int64(1), counterValue(t, reader, "microvault.loan.repayments",
		attribute.String("rail", railMoneyGram), attribute.String("outcome", models.LoanRepaymentStatusExpired)))
}
