package adapters

import (
	"context"
	"log/slog"

	"github.com/samber/oops"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mgpoller"
)

// Compile-time check.
var _ mgpoller.RepaymentNotifier = (*RepaymentNotifierAdapter)(nil)

// RepaymentNotifierAdapter turns the deposit driver's loan-ID-only notifier
// calls into fully populated loan notifications.
//
// The driver deliberately knows nothing about phone numbers, references or
// languages — it holds a projection, not a loan. This adapter is where the row
// is read back and the message assembled.
type RepaymentNotifierAdapter struct {
	repo   repository.LoanRepository
	loans  contracts.LoanNotifier
	logger *slog.Logger
}

// NewRepaymentNotifierAdapter builds the adapter. repo and notifier are
// required; logger may be nil.
func NewRepaymentNotifierAdapter(
	repo repository.LoanRepository,
	notifier contracts.LoanNotifier,
	logger *slog.Logger,
) (*RepaymentNotifierAdapter, error) {
	if repo == nil {
		return nil, oops.In(errDomain).
			Code("missing_dependency").
			With("dependency", "loan_repository").
			Errorf("required dependency is missing")
	}
	if notifier == nil {
		return nil, oops.In(errDomain).
			Code("missing_dependency").
			With("dependency", "loan_notifier").
			Errorf("required dependency is missing")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RepaymentNotifierAdapter{
		repo:   repo,
		loans:  notifier,
		logger: logger.With("component", "repayment_notifier"),
	}, nil
}

// NotifyRepaymentReceived confirms the borrower's cash reached the treasury.
//
// Sent while the treasury-to-vault leg may still be retrying. That is
// deliberate: from the borrower's side the repayment is complete, and the
// remaining leg is ours.
func (a *RepaymentNotifierAdapter) NotifyRepaymentReceived(loanID string) error {
	return a.send(loanID, "received", a.loans.NotifyRepaymentReceived)
}

// NotifyRepaymentReminder warns that an opened deposit is about to lapse.
//
// Routed to NotifyRepaymentWindowExpiring rather than the notifier's own
// NotifyRepaymentReminder, which is driven by the loan's due date and would
// tell the borrower the wrong thing entirely.
func (a *RepaymentNotifierAdapter) NotifyRepaymentReminder(loanID string) error {
	return a.send(loanID, "window_expiring", a.loans.NotifyRepaymentWindowExpiring)
}

// NotifyRepaymentExpired reports that a deposit lapsed unused.
func (a *RepaymentNotifierAdapter) NotifyRepaymentExpired(loanID string) error {
	return a.send(loanID, "expired", a.loans.NotifyRepaymentExpired)
}

// send loads the loan and hands a populated notification to one notifier
// method.
func (a *RepaymentNotifierAdapter) send(loanID, kind string, notify func(context.Context, contracts.LoanNotification) error) error {
	ctx := context.Background()

	errb := oops.In(errDomain).
		Tags("notification").
		With("loan_id", loanID).
		With("notification", kind)

	loanRow, err := a.repo.GetByID(ctx, loanID)
	if err != nil {
		return errb.Code("loan_load_failed").Wrapf(err, "could not load the loan")
	}

	n := contracts.LoanNotification{
		LoanID:             loanID,
		UserID:             loanRow.UserID,
		DisplayCurrency:    "USD",
		RepaymentExpiresAt: loanRow.RepaymentExpiresAt,
	}
	if loanRow.LoanReference != nil {
		n.LoanReference = *loanRow.LoanReference
	}
	if loanRow.User != nil {
		n.PhoneNumber = loanRow.User.MobileNumber
	}
	if loanRow.RepaymentPayoffStroops != nil {
		n.Amount = *loanRow.RepaymentPayoffStroops
		n.DisplayAmount = float64(*loanRow.RepaymentPayoffStroops) / 1e7
	}

	// No phone, no SMS. Worth its own error rather than a silent success: a
	// borrower who is never told their repayment landed will call support.
	if n.PhoneNumber == "" {
		return errb.Code("missing_phone_number").Errorf("loan has no phone number to notify")
	}

	if err := notify(ctx, n); err != nil {
		return errb.Code("send_failed").Wrapf(err, "could not send the notification")
	}
	return nil
}
