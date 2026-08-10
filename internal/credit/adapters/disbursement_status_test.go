package adapters

import (
	"testing"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/yellowcard"
	"github.com/stretchr/testify/assert"
)

// The two providers disagreed on the wire: YellowCard sends "complete",
// MoneyGram sends "completed". Everything downstream must see one vocabulary.
func TestCanonicalDisbursementStatus(t *testing.T) {
	assert.Equal(t, models.DisbursementStatusCompleted,
		canonicalDisbursementStatus(yellowcard.DisbursementComplete))
	assert.Equal(t, models.DisbursementStatusCompleted,
		canonicalDisbursementStatus(models.DisbursementStatusCompleted),
		"already-canonical values must pass through unchanged")
	assert.Equal(t, models.DisbursementStatusRefundReceived,
		canonicalDisbursementStatus(models.DisbursementStatusRefundReceived))
}

// A MoneyGram completion previously matched nothing, so the loan stayed
// "disbursing" — and a refunded loan stayed there too, counting as active
// against the borrower forever.
func TestLoanStatusForDisbursement(t *testing.T) {
	cases := map[string]string{
		models.DisbursementStatusCompleted:      models.LoanStatusDisbursed,
		models.DisbursementStatusFailed:         models.LoanStatusOffRampFailed,
		models.DisbursementStatusRefundReceived: models.LoanStatusCancelled,
		models.DisbursementStatusRefundPending:  "",
		models.DisbursementStatusProcessing:     "",
		models.DisbursementStatusMGInitiated:    "",
	}
	for disbursement, want := range cases {
		assert.Equal(t, want, loanStatusForDisbursement(disbursement),
			"disbursement status %q", disbursement)
	}
}

// A settled refund must leave the loan outside the active set. GetActiveLoans
// filters on approved/disbursing/disbursed, so cancelled is what removes it.
func TestRefundedLoanLeavesActiveSet(t *testing.T) {
	active := map[string]bool{
		models.LoanStatusApproved:   true,
		models.LoanStatusDisbursing: true,
		models.LoanStatusDisbursed:  true,
	}
	settled := loanStatusForDisbursement(models.DisbursementStatusRefundReceived)

	assert.False(t, active[settled],
		"a refunded borrower still owes nothing and must not hold an active loan")
}

// Canonicalisation runs before this mapping, so it must key off the model's
// vocabulary — matching YellowCard's "complete" here would silently leave the
// off_ramp transaction row unsettled.
func TestMapDisbursementToTxStatus(t *testing.T) {
	assert.Equal(t, "success", mapDisbursementToTxStatus(models.DisbursementStatusCompleted))
	assert.Equal(t, "failed", mapDisbursementToTxStatus(models.DisbursementStatusFailed))
	assert.Equal(t, "submitted", mapDisbursementToTxStatus(models.DisbursementStatusProcessing))
	assert.Empty(t, mapDisbursementToTxStatus(yellowcard.DisbursementComplete),
		"the raw wire value must not reach this function")
}
