package adapters

import (
	"context"
	"log/slog"
	"strings"

	"github.com/samber/oops"

	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mgpoller"
	"github.com/Shamba-Records-Limited/microvault/pkg/urlshortener"
)

// notifyDefaultCurrency is what a repayment SMS shows when a loan has no
// RampFiatCurr on file — every current corridor is Kenyan.
const notifyDefaultCurrency = "KES"

// Compile-time check.
var _ mgpoller.RepaymentNotifier = (*RepaymentNotifierAdapter)(nil)

// RepaymentNotifierAdapter turns the deposit driver's loan-ID-only notifier
// calls into fully populated loan notifications.
type RepaymentNotifierAdapter struct {
	repo          repository.LoanRepository
	loans         contracts.LoanNotifier
	publicBaseURL string
	shortener     urlshortener.Shortener
	offRamps      *offramp.Registry
	logger        *slog.Logger
}

// NewRepaymentNotifierAdapter builds the adapter. repo and notifier are
// required; logger may be nil. offRamps is optional — nil means every
// amount falls back to USDC, matching the previous behaviour, rather than
// failing to construct over what is only a display nicety.
func NewRepaymentNotifierAdapter(
	repo repository.LoanRepository,
	notifier contracts.LoanNotifier,
	publicBaseURL string,
	shortener urlshortener.Shortener,
	offRamps *offramp.Registry,
	logger *slog.Logger,
) (*RepaymentNotifierAdapter, error) {
	if repo == nil {
		return nil, oops.In(errDomain).
			Code(pkgErrors.CodeMissingDependency).
			With(pkgErrors.AttrDependency, "loan_repository").
			Errorf("required dependency is missing")
	}
	if notifier == nil {
		return nil, oops.In(errDomain).
			Code(pkgErrors.CodeMissingDependency).
			With(pkgErrors.AttrDependency, "loan_notifier").
			Errorf("required dependency is missing")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RepaymentNotifierAdapter{
		repo:          repo,
		loans:         notifier,
		publicBaseURL: strings.TrimSuffix(publicBaseURL, "/"),
		shortener:     shortener,
		offRamps:      offRamps,
		logger:        logger.With("component", "repayment_notifier"),
	}, nil
}

// NotifyRepaymentReference sends the code the borrower quotes at the counter.
func (a *RepaymentNotifierAdapter) NotifyRepaymentReference(ctx context.Context, loanID, reference string) error {
	return a.send(ctx, loanID, "reference", func(ctx context.Context, n contracts.LoanNotification) error {
		n.CashPickupRef = reference
		return a.loans.NotifyRepaymentReference(ctx, n)
	})
}

// NotifyRepaymentMoreInfo sends MoneyGram's transaction page when no reference
// has been issued.
//
// Errors rather than sending a message with an empty link: the whole point of
// this notification is the URL, and copy that trails off after "open this for
// your MoneyGram payment details:" tells the borrower nothing while consuming
// the one-shot send marker.
func (a *RepaymentNotifierAdapter) NotifyRepaymentMoreInfo(ctx context.Context, loanID string) error {
	return a.send(ctx, loanID, "more_info", func(ctx context.Context, n contracts.LoanNotification) error {
		if n.InteractiveURL == "" {
			return oops.In(errDomain).
				Code(pkgErrors.CodeIncompleteResponse).
				With(pkgErrors.AttrLoanID, loanID).
				Errorf("no transaction page to send")
		}
		return a.loans.NotifyRepaymentMoreInfo(ctx, n)
	})
}

// NotifyRepaymentReceived confirms the borrower's cash reached the treasury.
func (a *RepaymentNotifierAdapter) NotifyRepaymentReceived(ctx context.Context, loanID string) error {
	return a.sendAmount(ctx, loanID, "received", 0, a.loans.NotifyRepaymentReceived)
}

// NotifyRepaymentReceivedAmount is NotifyRepaymentReceived with the amount
// actually observed (e.g. an STK callback's AmountKES), which can differ
// from the loan's locked payoff. amountKES 0 falls back to the payoff.
func (a *RepaymentNotifierAdapter) NotifyRepaymentReceivedAmount(ctx context.Context, loanID string, amountKES int64) error {
	return a.sendAmount(ctx, loanID, "received", amountKES, a.loans.NotifyRepaymentReceived)
}

// NotifyLoanRepaid confirms the treasury-to-vault leg confirmed and the loan
// is closed.
func (a *RepaymentNotifierAdapter) NotifyLoanRepaid(ctx context.Context, loanID string) error {
	return a.sendAmount(ctx, loanID, "repaid", 0, a.loans.NotifyLoanRepaid)
}

// NotifyLoanRepaidAmount is NotifyLoanRepaid with the amount actually
// observed (e.g. an STK callback's AmountKES). amountKES 0 falls back to
// the payoff.
func (a *RepaymentNotifierAdapter) NotifyLoanRepaidAmount(ctx context.Context, loanID string, amountKES int64) error {
	return a.sendAmount(ctx, loanID, "repaid", amountKES, a.loans.NotifyLoanRepaid)
}

// NotifyRepaymentReminder warns that an opened deposit is about to lapse.
//
// Routed to NotifyRepaymentWindowExpiring rather than the notifier's own
// NotifyRepaymentReminder, which is driven by the loan's due date and would
// tell the borrower the wrong thing entirely.
func (a *RepaymentNotifierAdapter) NotifyRepaymentReminder(ctx context.Context, loanID string) error {
	return a.send(ctx, loanID, "window_expiring", a.loans.NotifyRepaymentWindowExpiring)
}

// NotifyRepaymentExpired reports that a deposit lapsed unused.
func (a *RepaymentNotifierAdapter) NotifyRepaymentExpired(ctx context.Context, loanID string) error {
	return a.send(ctx, loanID, "expired", a.loans.NotifyRepaymentExpired)
}

// moreInfoLink builds the SMS link to MoneyGram's transaction page.
func (a *RepaymentNotifierAdapter) moreInfoLink(ctx context.Context, loanRow *models.Loan) string {
	fallback := ""
	if a.publicBaseURL != "" && loanRow.RampMoreInfoShortCode != nil && *loanRow.RampMoreInfoShortCode != "" {
		fallback = a.publicBaseURL + "/r/" + *loanRow.RampMoreInfoShortCode
	}

	var rawURL string
	if loanRow.RampMoreInfoURL != nil {
		rawURL = *loanRow.RampMoreInfoURL
	}

	link, err := shortenedLink(ctx, a.shortener, rawURL, fallback)
	if err != nil {
		a.logger.WarnContext(ctx, "dub shorten failed; sending the redirect link instead",
			pkgErrors.AttrLoanID, loanRow.ID, "error", err)
	}
	return link
}

// send loads the loan and hands a populated notification to one notifier
// method.
func (a *RepaymentNotifierAdapter) send(ctx context.Context, loanID, kind string, notify func(context.Context, contracts.LoanNotification) error) error {
	return a.sendAmount(ctx, loanID, kind, 0, notify)
}

// sendAmount is send with an optional observed-amount override; see
// NotifyRepaymentReceivedAmount.
func (a *RepaymentNotifierAdapter) sendAmount(ctx context.Context, loanID, kind string, observedAmountKES int64, notify func(context.Context, contracts.LoanNotification) error) error {
	errb := oops.In(errDomain).
		Tags("notification").
		With(pkgErrors.AttrLoanID, loanID).
		With(pkgErrors.AttrNotification, kind)

	loanRow, err := a.repo.GetByID(ctx, loanID)
	if err != nil {
		return errb.Code(pkgErrors.CodeLoanLoadFailed).Wrapf(err, "could not load the loan")
	}

	n := contracts.LoanNotification{
		LoanID:             loanID,
		UserID:             loanRow.UserID,
		DisplayCurrency:    "USDC",
		RepaymentExpiresAt: loanRow.RepaymentExpiresAt,
		// The /r/{code} redirect, not MoneyGram's raw URL: that one carries a
		// JWT in its query string and would split a one-segment SMS into four.
		// RepaymentWindowExpiring reads this too, and rendered an empty line
		// where the link belongs until it was populated here.
		InteractiveURL: a.moreInfoLink(ctx, loanRow),
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

		received := int64(0)
		if loanRow.RepaymentReceivedStroops != nil {
			received = *loanRow.RepaymentReceivedStroops
		}
		remainingStroops := *loanRow.RepaymentPayoffStroops - received
		if remainingStroops < 0 {
			remainingStroops = 0
		}
		n.RemainingBalance = float64(remainingStroops) / 1e7

		currency := notifyDefaultCurrency
		if loanRow.RampFiatCurr != nil && *loanRow.RampFiatCurr != "" {
			currency = *loanRow.RampFiatCurr
		}
		if rate, _, err := offrampSellRate(ctx, a.offRamps, loanRow.RampProvider, currency); err == nil {
			n.DisplayCurrency = currency
			n.DisplayAmount = n.DisplayAmount * rate
			n.RemainingBalance = n.RemainingBalance * rate
		} else {
			a.logger.WarnContext(ctx, "no FX rate available to render a repayment notification in local currency, sending in USDC",
				pkgErrors.AttrLoanID, loanID, "error", err)
		}
	}
	if observedAmountKES > 0 {
		n.DisplayCurrency = notifyDefaultCurrency
		n.DisplayAmount = float64(observedAmountKES)
	}

	// No phone, no SMS. Worth its own error rather than a silent success: a
	// borrower who is never told their repayment landed will call support.
	if n.PhoneNumber == "" {
		return errb.Code(pkgErrors.CodeMissingPhoneNumber).Errorf("loan has no phone number to notify")
	}

	if err := notify(ctx, n); err != nil {
		return errb.Code(pkgErrors.CodeSendFailed).Wrapf(err, "could not send the notification")
	}
	return nil
}
