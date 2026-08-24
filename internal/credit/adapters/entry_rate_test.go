package adapters

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
)

type quotingProvider struct {
	id   offramp.ProviderID
	rate offramp.ExchangeRate
}

func (p *quotingProvider) ID() offramp.ProviderID { return p.id }

func (p *quotingProvider) Initiate(_ context.Context, _ offramp.Request) (*offramp.Result, error) {
	return &offramp.Result{RequestID: string(p.id)}, nil
}

func (p *quotingProvider) Quote(_ context.Context, _ offramp.QuoteRequest) (*offramp.ExchangeRate, error) {
	r := p.rate
	return &r, nil
}

type providerOptions struct{ id offramp.ProviderID }

func (o providerOptions) ProviderID() offramp.ProviderID { return o.id }

func newRateAdapter(t *testing.T, rate offramp.ExchangeRate) (*LoanServiceAdapter, providerOptions) {
	t.Helper()

	reg := offramp.NewRegistry()
	require.NoError(t, reg.Register(&quotingProvider{id: "yc", rate: rate}))

	return &LoanServiceAdapter{
		offRamps: reg,
		fxBuffer: offramp.NewRateBuffer(offramp.Fraction(0.015), DefaultFXBufferPct),
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, providerOptions{id: "yc"}
}

func TestRequoteEntryRate_UsesSellRate(t *testing.T) {
	a, opts := newRateAdapter(t, offramp.ExchangeRate{SellRate: 128.19, BuyRate: 124.00})

	rate, source, buffer := a.requoteEntryRate(context.Background(), opts, "KES", "KE")

	assert.InDelta(t, 126.2671, rate, 0.0001, "the 1.5%% buffer applies to the sell rate")
	assert.Equal(t, "yc", source)
	assert.InDelta(t, 0.015, buffer, 0.0001)
}

// A provider that omits the sell rate used to fall through to buy, quoting the
// borrower at the wrong side of the spread with nothing on the loan recording
// that it had happened.
func TestRequoteEntryRate_NeverFallsBackToBuy(t *testing.T) {
	a, opts := newRateAdapter(t, offramp.ExchangeRate{SellRate: 0, BuyRate: 124.00})

	rate, source, buffer := a.requoteEntryRate(context.Background(), opts, "KES", "KE")

	assert.Zero(t, rate, "no sell rate means no entry rate")
	assert.Empty(t, source)
	assert.Zero(t, buffer)
}
