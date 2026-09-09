package adapters

import (
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
)

func corridorCode(t *testing.T, err error) string {
	t.Helper()
	var oopsErr oops.OopsError
	require.ErrorAs(t, err, &oopsErr)
	code, _ := oopsErr.Code().(string)
	return code
}

// The ceiling had no check at all before this: a payoff of any size opened a
// deposit, and MoneyGram refused it at the counter after the quote was locked
// and the link had already gone out by SMS.
func TestDepositCorridorErr(t *testing.T) {
	errb := oops.In(pkgErrors.DomainRepaymentCashIn)

	cases := []struct {
		name     string
		stroops  int64
		wantCode string
	}{
		{"below the floor", ussd.MinMoneyGramDepositStroops - 1, pkgErrors.CodeBelowAnchorMinimum},
		{"exactly the floor", ussd.MinMoneyGramDepositStroops, ""},
		{"mid-range", 5_000_000_000, ""},
		{"exactly the ceiling", ussd.MaxMoneyGramDepositStroops, ""},
		{"above the ceiling", ussd.MaxMoneyGramDepositStroops + 1, pkgErrors.CodeAboveAnchorMaximum},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := depositCorridorErr(errb, c.stroops)
			if c.wantCode == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, c.wantCode, corridorCode(t, err))
		})
	}
}

// The two ends are actioned differently — mobile money below, split the
// payment above — so they must not collapse to one code on a dashboard.
func TestDepositCorridorErr_EndsAreDistinguishable(t *testing.T) {
	errb := oops.In(pkgErrors.DomainRepaymentCashIn)

	low := depositCorridorErr(errb, 1)
	high := depositCorridorErr(errb, 100_000_000_000)

	assert.NotEqual(t, corridorCode(t, low), corridorCode(t, high))
}
