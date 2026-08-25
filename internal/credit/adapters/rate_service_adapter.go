package adapters

import (
	"context"

	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/moneygram"

	"github.com/samber/oops"

	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
)

// Compile-time check.
var _ ussd.RateService = (*RateServiceAdapter)(nil)

// RateServiceAdapter implements ussd.RateService against MoneyGram's FX
// cascade — MG primary, YellowCard fallback, then the orchestrator's stale
// cache.
//
// The USSD flow converts the borrower's fiat amount to USDC at this rate and
// gates MoneyGram's cash-pickup minimum against it, so quoting from anywhere
// else would measure the anchor's floor with someone else's rate.
type RateServiceAdapter struct {
	orchestrator *moneygram.FXOrchestrator
}

// NewRateServiceAdapter creates a new RateServiceAdapter.
func NewRateServiceAdapter(orchestrator *moneygram.FXOrchestrator) *RateServiceAdapter {
	return &RateServiceAdapter{orchestrator: orchestrator}
}

// GetExchangeRate returns the buffered sell rate for the given currency
// (e.g. "KES"), in local units per USD.
//
// An unmapped currency leaves DestinationCountry empty, which makes the
// orchestrator skip MoneyGram and quote from the fallback instead of asking
// for a corridor that does not exist.
func (a *RateServiceAdapter) GetExchangeRate(ctx context.Context, currency string) (float64, error) {
	res, err := a.orchestrator.Quote(ctx, moneygram.FXQuoteRequest{
		OriginatingCountry: moneygram.DefaultOriginatingCountry,
		DestinationCountry: moneygram.CountryISO3ForCurrency(currency),
		SendCurrency:       moneygram.DefaultSendCurrency,
		ReceiveCurrency:    currency,
	})
	if err != nil {
		return 0, oops.In(pkgErrors.DomainOffRamp).Tags("rate").
			With(pkgErrors.AttrCurrency, currency).
			Code(pkgErrors.CodeRateUnavailable).
			Wrapf(err, "could not get an exchange rate")
	}
	if res.Rate <= 0 {
		return 0, oops.In(pkgErrors.DomainOffRamp).Tags("rate").
			With(pkgErrors.AttrCurrency, currency).
			With("source", res.Source).
			With("rate", res.Rate).
			Code(pkgErrors.CodeRateUnavailable).
			Errorf("rate source returned a non-positive rate")
	}
	return res.Rate, nil
}
