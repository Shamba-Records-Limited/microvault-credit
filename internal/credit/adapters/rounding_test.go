package adapters

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoundToCentStroops(t *testing.T) {
	tests := []struct {
		name    string
		stroops int64
		want    int64
	}{
		// MoneyGram incident cases: the anchor quoted 23.43 and 21.88 while we
		// sent the raw FX conversion on-chain.
		{"mg case 1: 23.430178 rounds down to 23.43", 234_301_780, 234_300_000},
		{"mg case 2: 21.8767091 rounds up to 21.88", 218_767_091, 218_800_000},

		{"exact cent unchanged", 500_000_000, 500_000_000},
		{"half cent rounds up", 150_000, 200_000},
		{"just below half rounds down", 149_999, 100_000},
		{"one stroop above cent rounds down", 100_001, 100_000},
		{"one stroop below next cent rounds up", 199_999, 200_000},

		// A positive sub-cent amount must never become a zero loan.
		{"tiny amount clamps to one cent", 1, 100_000},
		{"sub-half-cent clamps to one cent", 49_999, 100_000},

		{"zero stays zero", 0, 0},
		{"negative passes through", -5, -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, roundToCentStroops(tt.stroops))
		})
	}
}
