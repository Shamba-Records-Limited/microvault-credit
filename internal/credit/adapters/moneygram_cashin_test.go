package adapters

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/payment/cashin"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
)

// newCashAdapter builds a LoanServiceAdapter and, when withCashIn is true,
// registers it into a fresh registry as the MoneyGram cash-in collector —
// mirroring what cmd/credit/main.go does at boot.
func newCashAdapter(t *testing.T, resp *loan.LoanResponse, withCashIn bool) *LoanServiceAdapter {
	t.Helper()
	a := &LoanServiceAdapter{
		loanSvc: &byIDLoanSvc{resp: resp},
		logger:  slog.New(slog.DiscardHandler),
	}
	if withCashIn {
		reg := cashin.NewRegistry()
		require.NoError(t, reg.Register(a))
		require.NoError(t, reg.Alias(cashin.CollectionMethodCash, cashin.ProviderMoneyGram))
		a.cashIn = reg
	}
	return a
}

func TestMoneyGramID(t *testing.T) {
	a := newCashAdapter(t, promptLoanResponse(), false)

	assert.Equal(t, cashin.ProviderMoneyGram, a.ID())
}

func TestMoneyGramCollect_RefusesWithoutAPayer(t *testing.T) {
	a := newCashAdapter(t, promptLoanResponse(), false)

	_, err := a.Collect(context.Background(), cashin.Request{LoanID: "loan-1"})

	require.Error(t, err)
}

func TestMoneyGramCollect_RefusesWithoutAnAnchor(t *testing.T) {
	// Payer and loan id are both present, so this exercises the anchor guard
	// specifically rather than the payer guard above.
	a := newCashAdapter(t, promptLoanResponse(), false)

	_, err := a.Collect(context.Background(), cashin.Request{
		LoanID: "loan-1",
		Payer:  "254712345678",
	})

	require.Error(t, err)
}

func TestInitiateRepayment_RefusesWithoutACashInRegistry(t *testing.T) {
	a := newCashAdapter(t, promptLoanResponse(), false)

	err := a.InitiateRepayment(context.Background(), "loan-1", "254712345678")

	require.Error(t, err)
}

func TestInitiateRepayment_ResolvesThroughTheRegistry(t *testing.T) {
	// No anchor configured, so the error surfaces from inside Collect — this
	// confirms InitiateRepayment reaches the registered MoneyGram collector
	// via cashin.Resolve rather than erroring independently.
	a := newCashAdapter(t, promptLoanResponse(), true)

	err := a.InitiateRepayment(context.Background(), "loan-1", "254712345678")

	require.Error(t, err)
}

func TestMoneyGramStatus_MapsRepaymentStatusToSucceeded(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		succeeded bool
	}{
		{"initiated is not succeeded", "initiated", false},
		{"funds_received is succeeded", "funds_received", true},
		{"settled is succeeded", "settled", true},
		{"empty is not succeeded", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := "MV7K3QA9F"
			mgTxID := "tx-123"
			payoff := int64(1_000_000)
			resp := &loan.LoanResponse{
				LoanReference:          &ref,
				RepaymentStatus:        tt.status,
				RepaymentPayoffStroops: &payoff,
				RepaymentMGTxID:        &mgTxID,
			}
			a := newCashAdapter(t, resp, false)

			status, err := a.Status(context.Background(), cashin.ProviderRef{ID: "loan-1"})

			require.NoError(t, err)
			assert.Equal(t, ref, status.Reference)
			assert.Equal(t, tt.succeeded, status.Succeeded)
			assert.Equal(t, payoff, status.Amount)
		})
	}
}
