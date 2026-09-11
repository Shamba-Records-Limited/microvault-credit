package adapters

import (
	"context"

	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
)

// offrampSellRate resolves a sell-rate quote for currency, trying
// rampProvider's own Quoter first — it is the disbursing provider, so its
// rate is the one the borrower was already quoted against — and falling
// back to YellowCard's when rampProvider is unset, unregistered, or has no
// quoter. Shared by anything that needs to price an M-Pesa figure in a
// borrower's local currency: MpesaPaybillRepaymentDriver (converting a
// confirmed payment) and RepaymentNotifierAdapter (rendering an SMS amount).
//
// Sell only, never buy: pricing a repayment (or a repayment notification) at
// the buy rate is wrong by the spread, silently.
func offrampSellRate(ctx context.Context, offRamps *offramp.Registry, rampProvider *string, currency string) (rate float64, source string, err error) {
	if offRamps == nil {
		return 0, "", adapterErr("offramp_fx", "").Errorf("no offramp registry configured")
	}
	if rampProvider != nil && *rampProvider != "" {
		if rate, source, err := quoteSellRate(ctx, offRamps, offramp.ProviderID(*rampProvider), currency); err == nil {
			return rate, source, nil
		}
	}
	if rate, source, err := quoteSellRate(ctx, offRamps, offramp.ProviderYellowCard, currency); err == nil {
		return rate, source, nil
	}
	return 0, "", adapterErr("offramp_fx", "").
		With("ramp_provider", rampProvider).
		With("currency", currency).
		Errorf("no FX rate available from the disbursing provider or the YellowCard fallback")
}

func quoteSellRate(ctx context.Context, offRamps *offramp.Registry, id offramp.ProviderID, currency string) (float64, string, error) {
	provider, ok := offRamps.Get(id)
	if !ok {
		return 0, "", adapterErr("offramp_fx", "").With("provider", string(id)).Errorf("provider not registered")
	}
	quoter, ok := provider.(offramp.Quoter)
	if !ok {
		return 0, "", adapterErr("offramp_fx", "").With("provider", string(id)).Errorf("provider does not quote FX")
	}
	rate, err := quoter.Quote(ctx, offramp.QuoteRequest{Currency: currency})
	if err != nil {
		return 0, "", err
	}
	if rate.SellRate <= 0 {
		return 0, "", adapterErr("offramp_fx", "").With("provider", string(id)).Errorf("provider returned no sell rate")
	}
	return rate.SellRate, string(id), nil
}
