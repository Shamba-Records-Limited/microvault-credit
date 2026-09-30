package adapters

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/payment/cashin"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
)

type quoteStellar struct{ stellar.Service }

func (quoteStellar) GetBorrowIndex(context.Context) (int64, error) { return 1e18, nil }

type byIDLoanSvc struct {
	loan.Service
	resp *loan.LoanResponse
}

func (f *byIDLoanSvc) GetByID(context.Context, string) (*loan.LoanResponse, error) {
	return f.resp, nil
}

type fakePromptProvider struct {
	prompted []cashin.PromptRequest
}

func (f *fakePromptProvider) ID() cashin.ProviderID { return cashin.ProviderMpesa }

func (f *fakePromptProvider) Collect(context.Context, cashin.Request) (*cashin.Result, error) {
	return &cashin.Result{}, nil
}

func (f *fakePromptProvider) Prompt(_ context.Context, req cashin.PromptRequest) (*cashin.PromptResult, error) {
	f.prompted = append(f.prompted, req)
	return &cashin.PromptResult{}, nil
}

func newPromptAdapter(t *testing.T, resp *loan.LoanResponse, withCashIn bool) (*LoanServiceAdapter, *fakePromptProvider) {
	t.Helper()
	provider := &fakePromptProvider{}
	reg := offramp.NewRegistry()
	require.NoError(t, reg.Register(&quotingProvider{id: "yellowcard", rate: offramp.ExchangeRate{SellRate: 129}}))

	var cashReg *cashin.Registry
	if withCashIn {
		cashReg = cashin.NewRegistry()
		require.NoError(t, cashReg.Register(provider))
		require.NoError(t, cashReg.Alias(cashin.CollectionMethodPrompt, cashin.ProviderMpesa))
	}

	a := &LoanServiceAdapter{
		loanSvc:    &byIDLoanSvc{resp: resp},
		offRamps:   reg,
		fxBuffer:   offramp.NewRateBuffer(offramp.Fraction(0.015), DefaultFXBufferPct),
		logger:     slog.New(slog.DiscardHandler),
		stellarSvc: quoteStellar{},
		cashIn:     cashReg,
	}
	return a, provider
}

func promptLoanResponse() *loan.LoanResponse {
	index := int64(1e18)
	return &loan.LoanResponse{
		PrincipalAmount: 1_000_000,
		BorrowIndex:     &index,
	}
}

func TestPromptRepayment_PushesTheRoundedUpPayoff(t *testing.T) {
	a, provider := newPromptAdapter(t, promptLoanResponse(), true)

	// 0.1 USDC at 129 KES/USD is KES 12.90; M-Pesa takes whole shillings, so
	// the prompt is for 13.
	err := a.PromptRepayment(context.Background(), "loan-1", "254712345678")

	require.NoError(t, err)
	require.Len(t, provider.prompted, 1)
	assert.Equal(t, "loan-1", provider.prompted[0].LoanID)
	assert.Equal(t, "254712345678", provider.prompted[0].Payer)
	assert.Equal(t, int64(13), provider.prompted[0].AmountKES)
}

func TestPromptRepayment_RefusesWithoutACashInRegistry(t *testing.T) {
	a, _ := newPromptAdapter(t, promptLoanResponse(), false)

	err := a.PromptRepayment(context.Background(), "loan-1", "254712345678")

	require.Error(t, err)
}

func TestPromptRepayment_RefusesANonKESLoan(t *testing.T) {
	resp := promptLoanResponse()
	currency := "UGX"
	resp.RampFiatCurr = &currency
	a, provider := newPromptAdapter(t, resp, true)

	err := a.PromptRepayment(context.Background(), "loan-1", "254712345678")

	require.Error(t, err)
	assert.Empty(t, provider.prompted)
}
