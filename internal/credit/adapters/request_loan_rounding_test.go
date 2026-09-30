package adapters

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
)

type captureLoanSvc struct {
	loan.Service
	createdPrincipal int64
}

func (f *captureLoanSvc) Create(_ context.Context, req loan.CreateLoanRequest) (*loan.LoanResponse, error) {
	f.createdPrincipal = req.PrincipalAmount
	return &loan.LoanResponse{ID: "loan-1"}, nil
}

func (f *captureLoanSvc) Approve(context.Context, string, loan.ApproveLoanRequest) (*loan.LoanResponse, error) {
	return &loan.LoanResponse{ID: "loan-1"}, nil
}

func (f *captureLoanSvc) Update(context.Context, string, loan.UpdateLoanRequest) (*loan.LoanResponse, error) {
	return &loan.LoanResponse{ID: "loan-1"}, nil
}

func (f *captureLoanSvc) Disburse(context.Context, string, loan.DisburseLoanRequest) (*loan.LoanResponse, error) {
	return &loan.LoanResponse{ID: "loan-1"}, nil
}

type captureStellar struct {
	stellar.Service
	borrowAmount int64
}

func (f *captureStellar) GetBorrowAPR(context.Context) (int64, error) { return 0, nil }

func (f *captureStellar) BorrowFromVault(_ context.Context, req stellar.BorrowRequest) (*stellar.BorrowResponse, error) {
	f.borrowAmount = req.Amount
	return &stellar.BorrowResponse{TxHash: "tx", AmountBorrowed: req.Amount, BorrowIndex: 1}, nil
}

type captureProvider struct {
	id        offramp.ProviderID
	initiated offramp.Request
}

func (p *captureProvider) ID() offramp.ProviderID { return p.id }

func (p *captureProvider) Initiate(_ context.Context, req offramp.Request) (*offramp.Result, error) {
	p.initiated = req
	return &offramp.Result{RequestID: "req-1", SettlementMethod: "direct"}, nil
}

func (p *captureProvider) Quote(_ context.Context, _ offramp.QuoteRequest) (*offramp.ExchangeRate, error) {
	return &offramp.ExchangeRate{SellRate: 128.19, BuyRate: 124.00}, nil
}

func newRoundingAdapter(t *testing.T, round bool) (*LoanServiceAdapter, *captureLoanSvc, *captureStellar, *captureProvider) {
	t.Helper()

	prov := &captureProvider{id: offramp.ProviderYellowCard}
	reg := offramp.NewRegistry()
	require.NoError(t, reg.Register(prov))
	// MoneyGram resolves cash-pickup requests; registered so the unconditional
	// rounding test can drive that rail.
	require.NoError(t, reg.Register(&captureProvider{id: offramp.ProviderMoneyGram}))

	loans := &captureLoanSvc{}
	stel := &captureStellar{}
	a := &LoanServiceAdapter{
		loanSvc:       loans,
		stellarSvc:    stel,
		offRamps:      reg,
		logger:        slog.New(slog.DiscardHandler),
		productConfig: &ussd.LoanProductConfig{InterestRateBps: 500},
		fxBuffer:      offramp.NewRateBuffer(offramp.Fraction(0.02), DefaultFXBufferPct),
		dedupe:        newDedupeGate(60 * time.Second),

		roundAnchorAmounts: round,
	}
	return a, loans, stel, prov
}

// Rounding off (the default): a sub-cent principal reaches Create,
// BorrowFromVault, and Initiate with every stroop intact.
func TestRequestLoan_RoundingOffPassesSubCentThrough(t *testing.T) {
	a, loans, stel, prov := newRoundingAdapter(t, false)

	const principal int64 = 234_301_780
	_, err := a.RequestLoan(context.Background(), &ussd.LoanRequest{
		UserID:          "user-1",
		AccountID:       "acct-1",
		StellarAddress:  "GABC",
		PrincipalAmount: principal,
		PrincipalAsset:  "USDC",
	})
	require.NoError(t, err)

	assert.Equal(t, principal, loans.createdPrincipal, "Create")
	assert.Equal(t, principal, stel.borrowAmount, "BorrowFromVault")
	assert.Equal(t, principal, prov.initiated.AmountStroops, "Initiate")
}

// Rounding on: the mobile-money incident cases round to whole cents before
// any state mutates.
func TestRequestLoan_RoundingOnRoundsToWholeCents(t *testing.T) {
	cases := []struct {
		name      string
		principal int64
		want      int64
	}{
		{"23.430178 rounds down to 23.43", 234_301_780, 234_300_000},
		{"21.8767091 rounds up to 21.88", 218_767_091, 218_800_000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, loans, stel, prov := newRoundingAdapter(t, true)

			_, err := a.RequestLoan(context.Background(), &ussd.LoanRequest{
				UserID:          "user-1",
				AccountID:       "acct-1",
				StellarAddress:  "GABC",
				PrincipalAmount: tc.principal,
				PrincipalAsset:  "USDC",
			})
			require.NoError(t, err)

			assert.Equal(t, tc.want, loans.createdPrincipal, "Create")
			assert.Equal(t, tc.want, stel.borrowAmount, "BorrowFromVault")
			assert.Equal(t, tc.want, prov.initiated.AmountStroops, "Initiate")
		})
	}
}

// MoneyGram always reconciles at 2 decimals, so a cash-pickup principal rounds
// to whole cents even when the anchor-rounding toggle is off.
func TestRequestLoan_MoneyGramRoundsUnconditionally(t *testing.T) {
	cases := []struct {
		name      string
		principal int64
		want      int64
	}{
		{"23.430178 rounds down to 23.43", 234_301_780, 234_300_000},
		{"21.8767091 rounds up to 21.88", 218_767_091, 218_800_000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, loans, stel, _ := newRoundingAdapter(t, false) // toggle OFF

			_, err := a.RequestLoan(context.Background(), &ussd.LoanRequest{
				UserID:          "user-1",
				AccountID:       "acct-1",
				StellarAddress:  "GABC",
				PrincipalAmount: tc.principal,
				PrincipalAsset:  "USDC",
				PayoutMethod:    offramp.PayoutMethodCashPickup,
				RecipientName:   "Test User",
			})
			require.NoError(t, err)

			assert.Equal(t, tc.want, loans.createdPrincipal, "Create")
			assert.Equal(t, tc.want, stel.borrowAmount, "BorrowFromVault")
		})
	}
}
