package adapters

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/relay"
)

type stubRateSource struct {
	name  string
	rate  float64
	err   error
	calls int
}

func (s *stubRateSource) Name() string { return s.name }

func (s *stubRateSource) QuoteRate(_ context.Context, req relay.RateRequest) (*relay.RateQuote, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &relay.RateQuote{
		Provider:      s.name,
		Direction:     req.Direction,
		FiatCurrency:  req.FiatCurrency,
		CryptoAmount:  req.CryptoAmount,
		EffectiveRate: s.rate,
	}, nil
}

func routerFor(t *testing.T, enabled bool, sources ...relay.RateSource) *relay.Router {
	t.Helper()
	r, err := relay.NewWithSources(relay.Config{
		Enabled: enabled,
		Default: string(offramp.ProviderYellowCard),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, sources...)
	require.NoError(t, err)
	return r
}

func routingAdapter(router *relay.Router) *LoanServiceAdapter {
	return &LoanServiceAdapter{
		relayRouter: router,
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func mobileMoneyRequest() offramp.Request {
	return offramp.Request{
		LoanID:       "loan-1",
		CountryCode:  "KE",
		PayoutMethod: offramp.PayoutMethodMobileMoney,
	}
}

func TestRouteMobileMoney_PicksTheBetterProvider(t *testing.T) {
	yc := &stubRateSource{name: string(offramp.ProviderYellowCard), rate: 120}
	fb := &stubRateSource{name: string(offramp.ProviderFonbnk), rate: 123.4}

	got := routingAdapter(routerFor(t, true, yc, fb)).
		routeMobileMoney(context.Background(), mobileMoneyRequest(), 20)

	assert.Equal(t, PayoutMethodFonbnkMobileMoney, got, "the higher yield wins an off-ramp")
}

// Routing back to the default must leave the request alone rather than pin an
// alias, so the registry's own resolution is untouched.
func TestRouteMobileMoney_DefaultWinnerLeavesTheAlias(t *testing.T) {
	yc := &stubRateSource{name: string(offramp.ProviderYellowCard), rate: 130}
	fb := &stubRateSource{name: string(offramp.ProviderFonbnk), rate: 120}

	got := routingAdapter(routerFor(t, true, yc, fb)).
		routeMobileMoney(context.Background(), mobileMoneyRequest(), 20)

	assert.Equal(t, offramp.PayoutMethodMobileMoney, got)
}

// Cash pickup is a rail the borrower chose at the USSD menu, not a price
// decision. A better rate must never move them off collecting cash.
func TestRouteMobileMoney_NeverOverridesCashPickup(t *testing.T) {
	yc := &stubRateSource{name: string(offramp.ProviderYellowCard), rate: 120}
	fb := &stubRateSource{name: string(offramp.ProviderFonbnk), rate: 200}
	adapter := routingAdapter(routerFor(t, true, yc, fb))

	req := mobileMoneyRequest()
	req.PayoutMethod = offramp.PayoutMethodCashPickup

	assert.Empty(t, adapter.routeMobileMoney(context.Background(), req, 20))
	assert.Zero(t, fb.calls, "a cash-pickup loan must not even be quoted")
}

// Options pin a provider explicitly; the relay must not second-guess a caller
// that already chose one.
func TestRouteMobileMoney_NeverOverridesPinnedOptions(t *testing.T) {
	yc := &stubRateSource{name: string(offramp.ProviderYellowCard), rate: 120}
	fb := &stubRateSource{name: string(offramp.ProviderFonbnk), rate: 200}
	adapter := routingAdapter(routerFor(t, true, yc, fb))

	req := mobileMoneyRequest()
	req.Options = pinnedOptions{}

	assert.Empty(t, adapter.routeMobileMoney(context.Background(), req, 20))
	assert.Zero(t, fb.calls)
}

type pinnedOptions struct{}

func (pinnedOptions) ProviderID() offramp.ProviderID { return offramp.ProviderYellowCard }

func TestRouteMobileMoney_DisabledOrAbsentRouter(t *testing.T) {
	yc := &stubRateSource{name: string(offramp.ProviderYellowCard), rate: 120}
	fb := &stubRateSource{name: string(offramp.ProviderFonbnk), rate: 200}

	t.Run("switch off", func(t *testing.T) {
		adapter := routingAdapter(routerFor(t, false, yc, fb))
		assert.Empty(t, adapter.routeMobileMoney(context.Background(), mobileMoneyRequest(), 20))
	})

	t.Run("no router wired", func(t *testing.T) {
		adapter := routingAdapter(nil)
		assert.Empty(t, adapter.routeMobileMoney(context.Background(), mobileMoneyRequest(), 20))
	})
}

// A relay failure is not a loan failure — the registry's own alias still
// resolves and the borrower is disbursed through the default provider.
func TestRouteMobileMoney_FailureFallsBack(t *testing.T) {
	yc := &stubRateSource{name: string(offramp.ProviderYellowCard), err: errors.New("down")}
	fb := &stubRateSource{name: string(offramp.ProviderFonbnk), err: errors.New("down")}

	got := routingAdapter(routerFor(t, true, yc, fb)).
		routeMobileMoney(context.Background(), mobileMoneyRequest(), 20)

	assert.Empty(t, got, "an unroutable request keeps the registry's own dispatch")
}

// An unmapped winner cannot be resolved by the off-ramp registry, so it must
// fall back rather than pin an alias that does not exist.
func TestRouteMobileMoney_UnmappedProviderFallsBack(t *testing.T) {
	yc := &stubRateSource{name: string(offramp.ProviderYellowCard), rate: 120}
	stranger := &stubRateSource{name: "some-new-provider", rate: 500}

	got := routingAdapter(routerFor(t, true, yc, stranger)).
		routeMobileMoney(context.Background(), mobileMoneyRequest(), 20)

	assert.Empty(t, got, "a winner the off-ramp registry cannot resolve must not be pinned")
}
