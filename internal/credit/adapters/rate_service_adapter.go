package adapters

import (
	"context"
	"fmt"

	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	ussdadapters "github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd/adapters"
)

// Compile-time check.
var _ ussd.RateService = (*RateServiceAdapter)(nil)

// RateServiceAdapter implements ussd.RateService using the YellowCard off-ramp adapter's
// exchange rate endpoint.
type RateServiceAdapter struct {
	offRampSvc ussdadapters.OffRampService
}

// NewRateServiceAdapter creates a new RateServiceAdapter.
func NewRateServiceAdapter(offRampSvc ussdadapters.OffRampService) *RateServiceAdapter {
	return &RateServiceAdapter{offRampSvc: offRampSvc}
}

// GetExchangeRate returns the buy rate for the given currency (e.g. "KES").
func (a *RateServiceAdapter) GetExchangeRate(ctx context.Context, currency string) (float64, error) {
	rate, err := a.offRampSvc.GetExchangeRate(ctx, currency)
	if err != nil {
		return 0, fmt.Errorf("get exchange rate for %s: %w", currency, err)
	}
	if rate.BuyRate <= 0 {
		return 0, fmt.Errorf("invalid buy rate for %s: %.4f", currency, rate.BuyRate)
	}
	return rate.BuyRate, nil
}
