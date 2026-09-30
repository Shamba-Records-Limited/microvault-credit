package adapters

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/samber/oops"

	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	txmodels "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/mpesa"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mgpoller"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
)

// stkQuerier is the part of the express client the driver uses, so tests can
// stand in for Daraja without HTTP.
type stkQuerier interface {
	ExpressQuery(ctx context.Context, checkoutRequestID string, shortcode uint) (*mpesa.ExpressQueryResponse, error)
}

// stkRepaymentNotifier is the one notification the STK and paybill drivers
// send — narrower than mgpoller.RepaymentNotifier, which also carries
// MoneyGram-only concepts this rail has no use for. amountKES 0 means "no
// observed amount, fall back to the loan's payoff."
// adapters.RepaymentNotifierAdapter already satisfies this structurally, so
// the same instance wired for MoneyGram can be passed here too.
type stkRepaymentNotifier interface {
	NotifyRepaymentReceivedAmount(ctx context.Context, loanID string, amountKES int64) error
}

// MpesaSTKLoanDriver resolves loans whose repayment was initiated with an STK
// prompt. The callback path carries no bill reference, so attribution lives on
// the loan row: repayment_mpesa_checkout_id is what this driver asks Daraja
// about, and the settlement is written back onto the loan.
type MpesaSTKLoanDriver struct {
	client      stkQuerier
	mpesaRepo   corerepository.MpesaTransactionRepository
	repo        repository.LoanRepository
	loanSvc     loan.Service
	notifier    stkRepaymentNotifier
	logger      *slog.Logger
	shortcode   uint
	interval    time.Duration
	maxAttempts int
	now         func() time.Time
}

// MpesaSTKLoanDriverDeps are the collaborators the driver needs; all required
// except the logger and Notifier. A nil Notifier is tolerated the same way
// mgpoller.DepositDriver treats one — the state write is the fact of record
// and must never be blocked by an SMS failure; a missing or failing notifier
// only logs a warning.
type MpesaSTKLoanDriverDeps struct {
	Client    *mpesa.Client
	MpesaRepo corerepository.MpesaTransactionRepository
	Repo      repository.LoanRepository
	LoanSvc   loan.Service
	Notifier  stkRepaymentNotifier
	Config    config.MpesaConfig
	Logger    *slog.Logger
}

// NewMpesaSTKLoanDriver builds the driver.
func NewMpesaSTKLoanDriver(deps MpesaSTKLoanDriverDeps) (*MpesaSTKLoanDriver, error) {
	if deps.Client == nil || deps.MpesaRepo == nil || deps.Repo == nil || deps.LoanSvc == nil {
		return nil, oops.In(pkgErrors.DomainRepaymentCashIn).Tags("mpesa", "stk-poller").
			Code(pkgErrors.CodeMissingDependency).Errorf("client, both repositories and the loan service are required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	maxAttempts := deps.Config.STKMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &MpesaSTKLoanDriver{
		client:      deps.Client,
		mpesaRepo:   deps.MpesaRepo,
		repo:        deps.Repo,
		loanSvc:     deps.LoanSvc,
		notifier:    deps.Notifier,
		logger:      logger.With("component", "mpesa_stk_loan_driver"),
		shortcode:   deps.Config.CollectionShortcode,
		interval:    deps.Config.STKPollInterval,
		maxAttempts: maxAttempts,
		now:         time.Now,
	}, nil
}

// Drive processes one loan. A transport error means "not yet", never a
// failure; the attempt counter only moves on an answer from Daraja.
func (d *MpesaSTKLoanDriver) Drive(ctx context.Context, l *models.Loan) {
	if l == nil || l.RepaymentMpesaCheckoutID == nil || *l.RepaymentMpesaCheckoutID == "" {
		d.logger.ErrorContext(ctx, "stk repayment has no checkout id", "loan_id", loanIDOrEmpty(l))
		return
	}

	resp, err := d.client.ExpressQuery(ctx, *l.RepaymentMpesaCheckoutID, d.shortcode)
	if err != nil {
		d.logger.WarnContext(ctx, "stk query failed, retrying next interval",
			"loan_id", l.ID, "error", err)
		d.scheduleNext(ctx, l.ID, l.RepaymentSTKAttempts)
		return
	}

	code, outcome := resp.Outcome()
	switch {
	case code == 0:
		d.settle(ctx, l)
	case outcome.Retryable:
		d.retryOrExpire(ctx, l)
	default:
		d.logger.InfoContext(ctx, "stk repayment failed terminally",
			"loan_id", l.ID, "result_code", code, "outcome", outcome.Message)
		d.close(ctx, l.ID, models.LoanRepaymentStatusExpired)
	}
}

// settle marks the loan funds received and, when the callback row exists,
// confirms it against this loan. The express query is Daraja's own word that
// the payment completed, so the loan settles even when the callback was lost
// and left no row to confirm; reconciliation backfills the receipt then.
func (d *MpesaSTKLoanDriver) settle(ctx context.Context, l *models.Loan) {
	checkoutID := *l.RepaymentMpesaCheckoutID

	var receipt *string
	var amountKES int64
	obs, err := d.mpesaRepo.GetByCheckoutID(ctx, checkoutID)
	switch {
	case err == nil:
		if err := d.mpesaRepo.Confirm(ctx, obs.TransID, txmodels.MpesaConfirmViaSTKQuery, l.ID); err != nil {
			d.logger.WarnContext(ctx, "could not confirm the callback observation",
				"loan_id", l.ID, "trans_id", obs.TransID, "error", err)
		} else {
			receipt = &obs.TransID
			amountKES = obs.AmountKes
		}
	case errors.Is(err, corerepository.ErrMpesaNotFound):
		d.logger.WarnContext(ctx, "settling without a callback receipt; reconciliation must backfill",
			"loan_id", l.ID)
	default:
		d.logger.WarnContext(ctx, "could not look up the callback observation",
			"loan_id", l.ID, "error", err)
	}

	row, err := d.repo.GetByID(ctx, l.ID)
	if err != nil {
		d.logger.ErrorContext(ctx, "could not load the loan to settle", "loan_id", l.ID, "error", err)
		return
	}
	if row.RepaymentStatus != models.LoanRepaymentStatusInitiated {
		return
	}
	row.RepaymentStatus = models.LoanRepaymentStatusFundsReceived
	row.RepaymentMpesaTransID = receipt
	row.RepaymentNextPollAt = nil
	if row.RepaymentPayoffStroops != nil {
		received := *row.RepaymentPayoffStroops
		row.RepaymentReceivedStroops = &received
	}
	if err := d.repo.Update(ctx, row); err != nil {
		d.logger.ErrorContext(ctx, "could not mark the repayment funds received", "loan_id", l.ID, "error", err)
		return
	}
	d.notify(ctx, l.ID, amountKES)
}

// notify tells the borrower their payment landed. Best-effort: the state
// write above is already durable, so a missing notifier or a failed send
// only logs.
func (d *MpesaSTKLoanDriver) notify(ctx context.Context, loanID string, amountKES int64) {
	if d.notifier == nil {
		d.logger.WarnContext(ctx, "no repayment notifier configured, message not sent", "loan_id", loanID)
		return
	}
	if err := d.notifier.NotifyRepaymentReceivedAmount(ctx, loanID, amountKES); err != nil {
		d.logger.WarnContext(ctx, "failed to send repayment received notification", "loan_id", loanID, "error", err)
	}
}

// retryOrExpire parks a retryable answer for one more interval, until the
// attempt ceiling turns the prompt expired.
func (d *MpesaSTKLoanDriver) retryOrExpire(ctx context.Context, l *models.Loan) {
	attempts := l.RepaymentSTKAttempts + 1
	if attempts >= d.maxAttempts {
		d.logger.InfoContext(ctx, "stk repayment exhausted its attempts", "loan_id", l.ID, "attempts", attempts)
		d.close(ctx, l.ID, models.LoanRepaymentStatusExpired)
		return
	}
	d.scheduleNext(ctx, l.ID, attempts)
}

// scheduleNext parks the loan for one more interval via the partial-update
// path, which cannot clear columns but has nothing to clear here.
func (d *MpesaSTKLoanDriver) scheduleNext(ctx context.Context, loanID string, attempts int) {
	next := d.now().Add(d.interval)
	if _, err := d.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RepaymentSTKAttempts: &attempts,
		RepaymentNextPollAt:  &next,
	}); err != nil {
		d.logger.ErrorContext(ctx, "could not reschedule the stk poll", "loan_id", loanID, "error", err)
	}
}

// close writes a terminal repayment status and stops the polling schedule.
// Uses the repository directly because the partial-update path cannot write a
// NULL next_poll_at, and leaving it set would keep the loan in the due set.
func (d *MpesaSTKLoanDriver) close(ctx context.Context, loanID, status string) {
	row, err := d.repo.GetByID(ctx, loanID)
	if err != nil {
		d.logger.ErrorContext(ctx, "could not load the loan to close", "loan_id", loanID, "error", err)
		return
	}
	row.RepaymentStatus = status
	row.RepaymentNextPollAt = nil
	if err := d.repo.Update(ctx, row); err != nil {
		d.logger.ErrorContext(ctx, "could not write the terminal repayment status", "loan_id", loanID, "error", err)
	}
}

func loanIDOrEmpty(l *models.Loan) string {
	if l == nil {
		return ""
	}
	return l.ID
}

// NewMpesaSTKLoanRunner pairs the driver with the poller cadence.
func NewMpesaSTKLoanRunner(deps MpesaSTKLoanDriverDeps) (*mgpoller.Runner[*models.Loan], error) {
	driver, err := NewMpesaSTKLoanDriver(deps)
	if err != nil {
		return nil, err
	}
	return mgpoller.NewRunner[*models.Loan](mgpoller.RunnerDeps[*models.Loan]{
		Direction: "mpesa-stk-loans",
		Interval:  deps.Config.STKPollInterval,
		MaxBatch:  100,
		Fetcher: mgpoller.FetchFunc[*models.Loan](func(ctx context.Context, limit int) ([]*models.Loan, error) {
			return deps.Repo.GetDueSTKRepayments(ctx, limit)
		}),
		Driver: driver,
		Logger: deps.Logger,
		LoanID: func(l *models.Loan) string { return l.ID },
	}), nil
}
