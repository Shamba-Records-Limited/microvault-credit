package models

import (
	"testing"
	"time"
)

func mgLoan() *Loan {
	provider := "moneygram"
	return &Loan{Status: LoanStatusDisbursing, RampProvider: &provider}
}

func TestDeriveDisbursementStatus(t *testing.T) {
	now := time.Now()

	t.Run("initiated before anything happens", func(t *testing.T) {
		if got := mgLoan().DeriveDisbursementStatus(); got != DisbursementStatusMGInitiated {
			t.Fatalf("got %q, want %q", got, DisbursementStatusMGInitiated)
		}
	})

	t.Run("pickup ready once the borrower has been told", func(t *testing.T) {
		l := mgLoan()
		l.RampPickupReadyAt = &now
		if got := l.DeriveDisbursementStatus(); got != DisbursementStatusProcessing {
			t.Fatalf("got %q, want %q", got, DisbursementStatusProcessing)
		}
	})

	t.Run("declared refund outranks pickup ready", func(t *testing.T) {
		l := mgLoan()
		l.RampPickupReadyAt = &now
		l.RampRefundDeclaredAt = &now
		if got := l.DeriveDisbursementStatus(); got != DisbursementStatusRefundPending {
			t.Fatalf("got %q, want %q", got, DisbursementStatusRefundPending)
		}
	})

	t.Run("settled refund outranks the declaration", func(t *testing.T) {
		l := mgLoan()
		l.RampRefundDeclaredAt = &now
		l.Status = LoanStatusCancelled
		if got := l.DeriveDisbursementStatus(); got != DisbursementStatusRefundReceived {
			t.Fatalf("got %q, want %q", got, DisbursementStatusRefundReceived)
		}
	})

	t.Run("terminal statuses win over markers", func(t *testing.T) {
		for status, want := range map[string]string{
			LoanStatusDisbursed:     DisbursementStatusCompleted,
			LoanStatusOffRampFailed: DisbursementStatusFailed,
		} {
			l := mgLoan()
			l.RampPickupReadyAt = &now
			l.Status = status
			if got := l.DeriveDisbursementStatus(); got != want {
				t.Fatalf("status %q: got %q, want %q", status, got, want)
			}
		}
	})
}

// The poller sends the cash-pickup SMS only while the derived status is not yet
// "processing", so the marker must not appear before the send is attempted —
// otherwise the borrower never receives their reference number. Writing
// ramp_pickup_ready_at is what the poller does immediately before notifying.
func TestPickupReadyGuardsTheSMSExactlyOnce(t *testing.T) {
	l := mgLoan()

	if l.DeriveDisbursementStatus() == DisbursementStatusProcessing {
		t.Fatal("first observation must not look already-notified")
	}

	now := time.Now()
	l.RampPickupReadyAt = &now

	if l.DeriveDisbursementStatus() != DisbursementStatusProcessing {
		t.Fatal("after the marker is stamped the SMS must not be re-sent")
	}
}

func TestIsDisbursementTerminal(t *testing.T) {
	terminal := []string{LoanStatusDisbursed, LoanStatusOffRampFailed,
		LoanStatusCancelled, LoanStatusRepaid, LoanStatusDefaulted}
	for _, s := range terminal {
		if l := (&Loan{Status: s}); !l.IsDisbursementTerminal() {
			t.Errorf("%q should be terminal", s)
		}
	}

	// A declared-but-unsettled refund is not terminal: the MoneyGram poller is
	// the only thing that will settle it, so it must keep being polled.
	now := time.Now()
	l := mgLoan()
	l.RampRefundDeclaredAt = &now
	if l.IsDisbursementTerminal() {
		t.Error("a declared refund awaiting inbound USDC must stay in flight")
	}
}
