package adapters

import (
	"context"
	"fmt"

	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
)

// Compile-time check.
var _ ussd.RateService = (*RateServiceAdapter)(nil)

// RateServiceAdapter implements ussd.RateService against any offramp.Quoter
// (e.g. the YellowCard adapter).
type RateServiceAdapter struct {
	quoter offramp.Quoter
}

// NewRateServiceAdapter creates a new RateServiceAdapter.
func NewRateServiceAdapter(quoter offramp.Quoter) *RateServiceAdapter {
	return &RateServiceAdapter{quoter: quoter}
}

// GetExchangeRate returns the buy rate for the given currency (e.g. "KES").
func (a *RateServiceAdapter) GetExchangeRate(ctx context.Context, currency string) (float64, error) {
	rate, err := a.quoter.Quote(ctx, offramp.QuoteRequest{Currency: currency})
	if err != nil {
		return 0, fmt.Errorf("get exchange rate for %s: %w", currency, err)
	}
	if rate.BuyRate <= 0 {
		return 0, fmt.Errorf("invalid buy rate for %s: %.4f", currency, rate.BuyRate)
	}
	return rate.BuyRate, nil
}
