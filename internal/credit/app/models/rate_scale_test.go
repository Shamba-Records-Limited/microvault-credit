package models

import "testing"

// An FX rate must survive the round trip at the precision NUMERIC(20,8) held,
// which is what the E8 scale replaces.
func TestRateE8RoundTrip(t *testing.T) {
	for _, rate := range []float64{128.23, 129.45678901, 0.00000001, 1, 12345.6789} {
		got := RateFromE8(RateE8(rate))
		if diff := got - rate; diff > 1e-8 || diff < -1e-8 {
			t.Errorf("RateFromE8(RateE8(%v)) = %v, drifted by %v", rate, got, diff)
		}
	}
}

// An unknown rate stays NULL rather than being recorded as a real zero rate.
func TestRateE8RejectsNonPositive(t *testing.T) {
	for _, rate := range []float64{0, -1, -0.5} {
		if got := RateE8(rate); got != nil {
			t.Errorf("RateE8(%v) = %v, want nil so the column stays NULL", rate, *got)
		}
	}
	if got := RateFromE8(nil); got != 0 {
		t.Errorf("RateFromE8(nil) = %v, want 0", got)
	}
}

func TestRateE8Rounds(t *testing.T) {
	// 1e-9 is half a unit below the scale's resolution and must round up.
	if got := *RateE8(1.000000005); got != 100000001 {
		t.Errorf("RateE8(1.000000005) = %d, want 100000001", got)
	}
}

// The buffer is a genuine percentage, so basis points are exact for it.
func TestBufferBps(t *testing.T) {
	cases := map[float64]int32{0.01: 100, 0.015: 150, 0.0001: 1, 1: 10000}
	for fraction, want := range cases {
		got := BufferBps(fraction)
		if got == nil || *got != want {
			t.Errorf("BufferBps(%v) = %v, want %d", fraction, got, want)
		}
		if back := BufferFraction(got); back != fraction {
			t.Errorf("BufferFraction(BufferBps(%v)) = %v", fraction, back)
		}
	}
}

func TestBufferBpsRejectsNonPositive(t *testing.T) {
	for _, fraction := range []float64{0, -0.01} {
		if got := BufferBps(fraction); got != nil {
			t.Errorf("BufferBps(%v) = %v, want nil", fraction, *got)
		}
	}
	if got := BufferFraction(nil); got != 0 {
		t.Errorf("BufferFraction(nil) = %v, want 0", got)
	}
}

// The defaults the FX orchestrator ships must land on exact basis points, or
// every loan records a buffer that disagrees with the one actually applied.
func TestOrchestratorDefaultsAreExactBps(t *testing.T) {
	for fraction, want := range map[float64]int32{0.01: 100, 0.015: 150} {
		if got := BufferBps(fraction); got == nil || *got != want {
			t.Errorf("default buffer %v does not map cleanly to bps: got %v want %d", fraction, got, want)
		}
	}
}
