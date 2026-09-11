package adapters

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	coremodels "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
)

type fakePaybillMpesaRepo struct {
	corerepository.MpesaTransactionRepository
	unapplied []*coremodels.MpesaTransaction
	applied   map[string]int64
	sums      map[string]int64
}

func (f *fakePaybillMpesaRepo) ListUnappliedConfirmed(_ context.Context, _ int) ([]*coremodels.MpesaTransaction, error) {
	return f.unapplied, nil
}

func (f *fakePaybillMpesaRepo) SetAppliedStroops(_ context.Context, id string, stroops int64) error {
	if f.applied == nil {
		f.applied = map[string]int64{}
	}
	f.applied[id] = stroops
	return nil
}

func (f *fakePaybillMpesaRepo) SumAppliedStroopsByLoan(_ context.Context, loanID string) (int64, error) {
	return f.sums[loanID], nil
}

type fakePaybillLoanRepo struct {
	repository.LoanRepository
	loans   map[string]*models.Loan
	updates []*models.Loan
}

func (f *fakePaybillLoanRepo) GetByID(_ context.Context, id string) (*models.Loan, error) {
	return f.loans[id], nil
}

func (f *fakePaybillLoanRepo) Update(_ context.Context, l *models.Loan) error {
	f.updates = append(f.updates, l)
	f.loans[l.ID] = l
	return nil
}

type fakeQuoter struct {
	stroops int64
	err     error
}

func (f *fakeQuoter) GetRepaymentQuote(_ context.Context, loanID string) (*ussd.RepaymentQuote, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &ussd.RepaymentQuote{LoanID: loanID, AmountUSDCStroops: f.stroops}, nil
}

type fakeQuoteProvider struct {
	id   offramp.ProviderID
	rate float64
	err  error
}

func (f *fakeQuoteProvider) ID() offramp.ProviderID { return f.id }

func (f *fakeQuoteProvider) Initiate(context.Context, offramp.Request) (*offramp.Result, error) {
	return nil, errors.New("not used in these tests")
}

func (f *fakeQuoteProvider) Quote(_ context.Context, _ offramp.QuoteRequest) (*offramp.ExchangeRate, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &offramp.ExchangeRate{SellRate: f.rate}, nil
}

func paybillLoan(id string) *models.Loan {
	rampProvider := "yellowcard"
	return &models.Loan{
		ID:              id,
		Status:          models.LoanStatusDisbursed,
		RepaymentStatus: models.LoanRepaymentStatusNone,
		RampProvider:    &rampProvider,
		LoanReference:   nil,
	}
}

func paybillTx(id, loanID string, amountKes int64) *coremodels.MpesaTransaction {
	return &coremodels.MpesaTransaction{ID: id, LoanID: &loanID, AmountKes: amountKes}
}

func newTestPaybillDriver(t *testing.T, m *fakePaybillMpesaRepo, r *fakePaybillLoanRepo, q *fakeQuoter, offRamps *offramp.Registry) *MpesaPaybillRepaymentDriver {
	t.Helper()
	return &MpesaPaybillRepaymentDriver{
		mpesaRepo: m,
		repo:      r,
		quoter:    q,
		offRamps:  offRamps,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		interval:  time.Minute,
	}
}

func registryWithYellowCard(t *testing.T, rate float64) *offramp.Registry {
	t.Helper()
	reg := offramp.NewRegistry()
	require.NoError(t, reg.Register(&fakeQuoteProvider{id: offramp.ProviderYellowCard, rate: rate}))
	return reg
}

func TestNewMpesaPaybillRepaymentDriver_MissingDependency(t *testing.T) {
	_, err := NewMpesaPaybillRepaymentDriver(MpesaPaybillRepaymentDriverDeps{})
	require.Error(t, err)
}

func TestPaybillTick_ConvertsAndAdvancesToPartial(t *testing.T) {
	loan := paybillLoan("loan-1")
	m := &fakePaybillMpesaRepo{
		unapplied: []*coremodels.MpesaTransaction{paybillTx("tx-1", "loan-1", 100_00)}, // 100 KES, minor units
		sums:      map[string]int64{},
	}
	r := &fakePaybillLoanRepo{loans: map[string]*models.Loan{"loan-1": loan}}
	q := &fakeQuoter{stroops: 10_000_000} // payoff far above one partial payment
	offRamps := registryWithYellowCard(t, 129.0)
	d := newTestPaybillDriver(t, m, r, q, offRamps)

	d.tick(context.Background())

	require.Len(t, m.applied, 1)
	stroops := m.applied["tx-1"]
	assert.Greater(t, stroops, int64(0))
	// Two writes: applyOne's lazy payoff lock (this loan had none), then
	// tick's own recompute pass. The fake's SumAppliedStroopsByLoan is a
	// static stub rather than a real aggregate over applied, so recompute
	// sees the loan's total as still zero here — this asserts recompute ran
	// and wrote a status, not that the sum reflects the conversion above
	// (the real repository's aggregate query is what makes that connection
	// in production).
	require.Len(t, r.updates, 2)
	assert.Equal(t, models.LoanRepaymentStatusPartialFundsReceived, r.updates[len(r.updates)-1].RepaymentStatus)
}

func TestPaybillTick_RecomputesFromTheAggregate(t *testing.T) {
	// Isolates recompute's own behavior against a sum the fake reports
	// directly — the aggregate query itself is exercised by the real
	// repository, not this unit test.
	loan := paybillLoan("loan-1")
	m := &fakePaybillMpesaRepo{sums: map[string]int64{"loan-1": 775_193}}
	r := &fakePaybillLoanRepo{loans: map[string]*models.Loan{"loan-1": loan}}
	q := &fakeQuoter{}
	d := newTestPaybillDriver(t, m, r, q, offramp.NewRegistry())
	payoff := int64(10_000_000)
	loan.RepaymentPayoffStroops = &payoff

	d.recompute(context.Background(), "loan-1")

	require.Len(t, r.updates, 1)
	assert.Equal(t, models.LoanRepaymentStatusPartialFundsReceived, r.updates[0].RepaymentStatus)
	require.NotNil(t, r.updates[0].RepaymentReceivedStroops)
	assert.Equal(t, int64(775_193), *r.updates[0].RepaymentReceivedStroops)
}

func TestPaybillRecompute_ReachingPayoffNotifiesOnce(t *testing.T) {
	payoff := int64(1_000_000)
	loan := paybillLoan("loan-1")
	loan.RepaymentPayoffStroops = &payoff
	loan.RepaymentStatus = models.LoanRepaymentStatusPartialFundsReceived

	m := &fakePaybillMpesaRepo{sums: map[string]int64{"loan-1": payoff}}
	r := &fakePaybillLoanRepo{loans: map[string]*models.Loan{"loan-1": loan}}
	q := &fakeQuoter{}
	n := &fakeSTKNotifier{}
	d := newTestPaybillDriver(t, m, r, q, offramp.NewRegistry())
	d.notifier = n

	d.recompute(context.Background(), "loan-1")

	require.Len(t, r.updates, 1)
	assert.Equal(t, models.LoanRepaymentStatusFundsReceived, r.updates[0].RepaymentStatus)
	assert.Equal(t, []string{"loan-1"}, n.notified)

	// A second recompute at the same total must not notify again.
	d.recompute(context.Background(), "loan-1")
	assert.Equal(t, []string{"loan-1"}, n.notified, "must not notify twice for the same loan reaching funds_received")
}

func TestPaybillApplyOne_IneligibleLoanIsSkipped(t *testing.T) {
	loan := paybillLoan("loan-1")
	loan.Status = models.LoanStatusRepaid
	m := &fakePaybillMpesaRepo{}
	r := &fakePaybillLoanRepo{loans: map[string]*models.Loan{"loan-1": loan}}
	q := &fakeQuoter{stroops: 1}
	offRamps := registryWithYellowCard(t, 129.0)
	d := newTestPaybillDriver(t, m, r, q, offRamps)

	applied := d.applyOne(context.Background(), paybillTx("tx-1", "loan-1", 100_00))

	assert.False(t, applied)
	assert.Empty(t, m.applied)
}

func TestPaybillApplyOne_LazyLocksPayoffOnFirstPayment(t *testing.T) {
	loan := paybillLoan("loan-1")
	m := &fakePaybillMpesaRepo{}
	r := &fakePaybillLoanRepo{loans: map[string]*models.Loan{"loan-1": loan}}
	q := &fakeQuoter{stroops: 5_000_000}
	offRamps := registryWithYellowCard(t, 129.0)
	d := newTestPaybillDriver(t, m, r, q, offRamps)

	applied := d.applyOne(context.Background(), paybillTx("tx-1", "loan-1", 100_00))

	require.True(t, applied)
	require.NotEmpty(t, r.updates)
	require.NotNil(t, r.updates[0].RepaymentPayoffStroops)
	assert.Equal(t, int64(5_000_000), *r.updates[0].RepaymentPayoffStroops)
	assert.Equal(t, models.LoanRepaymentProviderMpesa, r.updates[0].RepaymentProvider)
}

func TestPaybillApplyOne_FallsBackToYellowCardWhenDisbursementProviderHasNoRate(t *testing.T) {
	loan := paybillLoan("loan-1")
	fonbnk := "fonbnk"
	loan.RampProvider = &fonbnk
	loan.RepaymentPayoffStroops = new(int64)
	*loan.RepaymentPayoffStroops = 10_000_000

	m := &fakePaybillMpesaRepo{}
	r := &fakePaybillLoanRepo{loans: map[string]*models.Loan{"loan-1": loan}}
	q := &fakeQuoter{}
	offRamps := offramp.NewRegistry()
	require.NoError(t, offRamps.Register(&fakeQuoteProvider{id: offramp.ProviderYellowCard, rate: 129.0}))
	// fonbnk is not registered at all, forcing the fallback.
	d := newTestPaybillDriver(t, m, r, q, offRamps)

	applied := d.applyOne(context.Background(), paybillTx("tx-1", "loan-1", 100_00))

	require.True(t, applied)
	require.Len(t, m.applied, 1)
	assert.Greater(t, m.applied["tx-1"], int64(0))
}

func TestPaybillApplyOne_NoFXAvailableIsRetried(t *testing.T) {
	loan := paybillLoan("loan-1")
	loan.RepaymentPayoffStroops = new(int64)
	*loan.RepaymentPayoffStroops = 10_000_000

	m := &fakePaybillMpesaRepo{}
	r := &fakePaybillLoanRepo{loans: map[string]*models.Loan{"loan-1": loan}}
	q := &fakeQuoter{}
	offRamps := offramp.NewRegistry() // nobody registered — no FX anywhere
	d := newTestPaybillDriver(t, m, r, q, offRamps)

	applied := d.applyOne(context.Background(), paybillTx("tx-1", "loan-1", 100_00))

	assert.False(t, applied)
	assert.Empty(t, m.applied)
}
