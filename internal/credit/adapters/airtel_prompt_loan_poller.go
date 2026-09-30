package adapters

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/samber/lo"
	"github.com/samber/oops"

	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	coremodels "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/airtel"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mgpoller"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
)

// airtelEnquirer is the part of the client the driver uses, so tests can
// stand in for Airtel without HTTP.
type airtelEnquirer interface {
	Enquiry(ctx context.Context, transactionID string) (*airtel.EnquiryResponse, error)
}

// AirtelPromptLoanDriver resolves loans whose repayment was initiated with an
// Airtel USSD push.
//
// It correlates more directly than its M-Pesa counterpart. The transaction id
// on the loan row is the one we minted and sent, and it is also the staging
// table's key — so the callback row and the loan find each other without the
// receipt, which on this rail does not exist until the payment succeeds.
//
// The enquiry is the authority. A callback may have already reported TS or
// TF, but Airtel documents those as possibly intermediate, so nothing here
// trusts one: the loan settles on what the enquiry says.
type AirtelPromptLoanDriver struct {
	client      airtelEnquirer
	airtelRepo  corerepository.AirtelTransactionRepository
	repo        repository.LoanRepository
	loanSvc     loan.Service
	notifier    stkRepaymentNotifier
	logger      *slog.Logger
	interval    time.Duration
	maxAttempts int
	now         func() time.Time
}

// AirtelPromptLoanDriverDeps are the collaborators the driver needs; all
// required except Notifier and Logger. A nil Notifier is tolerated the same
// way the M-Pesa drivers treat one — the state write is the fact of record
// and must never be blocked by an SMS failure.
type AirtelPromptLoanDriverDeps struct {
	Client     *airtel.Client
	AirtelRepo corerepository.AirtelTransactionRepository
	Repo       repository.LoanRepository
	LoanSvc    loan.Service
	Notifier   stkRepaymentNotifier
	Config     config.AirtelConfig
	Logger     *slog.Logger
}

// NewAirtelPromptLoanDriver builds the driver.
func NewAirtelPromptLoanDriver(deps AirtelPromptLoanDriverDeps) (*AirtelPromptLoanDriver, error) {
	if deps.Client == nil || deps.AirtelRepo == nil || deps.Repo == nil || deps.LoanSvc == nil {
		return nil, oops.In(pkgErrors.DomainRepaymentCashIn).Tags("airtel", "prompt-poller").
			Code(pkgErrors.CodeMissingDependency).
			Errorf("client, both repositories and the loan service are required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	maxAttempts := deps.Config.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	interval := deps.Config.PollInterval
	if interval <= 0 {
		interval = time.Minute
	}
	return &AirtelPromptLoanDriver{
		client:      deps.Client,
		airtelRepo:  deps.AirtelRepo,
		repo:        deps.Repo,
		loanSvc:     deps.LoanSvc,
		notifier:    deps.Notifier,
		logger:      logger.With("component", "airtel_prompt_loan_driver"),
		interval:    interval,
		maxAttempts: maxAttempts,
		now:         time.Now,
	}, nil
}

// Drive processes one loan. A transport error means "not yet", never a
// failure; the attempt counter only moves on an answer from Airtel.
func (d *AirtelPromptLoanDriver) Drive(ctx context.Context, l *models.Loan) {
	if l == nil || l.RepaymentAirtelTxnID == nil || *l.RepaymentAirtelTxnID == "" {
		d.logger.ErrorContext(ctx, "airtel repayment has no transaction id", "loan_id", loanIDOrEmpty(l))
		return
	}

	resp, err := d.client.Enquiry(ctx, *l.RepaymentAirtelTxnID)
	if err != nil {
		d.logger.WarnContext(ctx, "airtel enquiry failed, retrying next interval",
			"loan_id", l.ID, "error", err)
		d.scheduleNext(ctx, l.ID, l.RepaymentAirtelAttempts)
		return
	}

	status := resp.TransactionStatus()
	switch {
	case status.Succeeded():
		d.settle(ctx, l, resp)
	case !status.Terminal(airtel.SourceEnquiry):
		// Ambiguous or in progress: the payer may still be entering a PIN.
		d.retryOrExpire(ctx, l)
	default:
		d.logger.InfoContext(ctx, "airtel repayment failed terminally",
			"loan_id", l.ID, "status", string(status), "message", resp.Data.Transaction.Message)
		d.close(ctx, l.ID, models.LoanRepaymentStatusExpired)
	}
}

// settle marks the loan funds received and confirms the staging row against
// it. The enquiry is Airtel's own word that the payment completed, so the
// loan settles even when no callback ever arrived and there is no row to
// confirm; the summary sweep backfills that.
func (d *AirtelPromptLoanDriver) settle(ctx context.Context, l *models.Loan, resp *airtel.EnquiryResponse) {
	transactionID := *l.RepaymentAirtelTxnID

	receipt, ok := resp.Receipt()
	if !ok {
		// Airtel reported success but disclosed no receipt. That contradicts
		// its own documented invariant, so it is worth a loud line — but the
		// payment still completed, and the loan still settles.
		d.logger.WarnContext(ctx, "airtel reported success with no receipt",
			"loan_id", l.ID, "transaction_id", transactionID)
	}

	var amountKES int64
	obs, err := d.airtelRepo.GetByPartnerID(ctx, transactionID)
	switch {
	case err == nil:
		amountKES = obs.AmountMinor / 100
		if err := d.airtelRepo.Confirm(ctx, transactionID, coremodels.AirtelConfirmViaEnquiry,
			string(resp.TransactionStatus()), receipt, l.ID); err != nil {
			d.logger.WarnContext(ctx, "could not confirm the callback observation",
				"loan_id", l.ID, "transaction_id", transactionID, "error", err)
		}
	case errors.Is(err, corerepository.ErrAirtelNotFound):
		d.logger.WarnContext(ctx, "settling without a staged callback; reconciliation must backfill",
			"loan_id", l.ID, "transaction_id", transactionID)
	default:
		d.logger.WarnContext(ctx, "could not look up the staged callback",
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
	row.RepaymentNextPollAt = nil
	if receipt != "" {
		row.RepaymentAirtelMoneyID = lo.ToPtr(receipt)
	}
	if row.RepaymentPayoffStroops != nil {
		row.RepaymentReceivedStroops = lo.ToPtr(*row.RepaymentPayoffStroops)
	}
	if err := d.repo.Update(ctx, row); err != nil {
		d.logger.ErrorContext(ctx, "could not mark the repayment funds received", "loan_id", l.ID, "error", err)
		return
	}
	d.notify(ctx, l.ID, amountKES)
}

// notify tells the borrower their payment landed. Best-effort: the state
// write above is already durable.
func (d *AirtelPromptLoanDriver) notify(ctx context.Context, loanID string, amountKES int64) {
	if d.notifier == nil {
		d.logger.WarnContext(ctx, "no repayment notifier configured, message not sent", "loan_id", loanID)
		return
	}
	if err := d.notifier.NotifyRepaymentReceivedAmount(ctx, loanID, amountKES); err != nil {
		d.logger.WarnContext(ctx, "failed to send repayment received notification", "loan_id", loanID, "error", err)
	}
}

// retryOrExpire parks a non-terminal answer for one more interval, until the
// attempt ceiling turns the prompt expired.
func (d *AirtelPromptLoanDriver) retryOrExpire(ctx context.Context, l *models.Loan) {
	attempts := l.RepaymentAirtelAttempts + 1
	if attempts >= d.maxAttempts {
		d.logger.InfoContext(ctx, "airtel repayment exhausted its attempts", "loan_id", l.ID, "attempts", attempts)
		d.close(ctx, l.ID, models.LoanRepaymentStatusExpired)
		return
	}
	d.scheduleNext(ctx, l.ID, attempts)
}

// scheduleNext parks the loan for one more interval.
func (d *AirtelPromptLoanDriver) scheduleNext(ctx context.Context, loanID string, attempts int) {
	next := d.now().Add(d.interval)
	if _, err := d.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RepaymentAirtelAttempts: &attempts,
		RepaymentNextPollAt:     &next,
	}); err != nil {
		d.logger.ErrorContext(ctx, "could not reschedule the airtel enquiry", "loan_id", loanID, "error", err)
	}
}

// close writes a terminal repayment status and stops the polling schedule.
// Uses the repository directly because the partial-update path cannot write a
// NULL next_poll_at, and leaving it set would keep the loan in the due set.
func (d *AirtelPromptLoanDriver) close(ctx context.Context, loanID, status string) {
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

// NewAirtelPromptLoanRunner pairs the driver with the poller cadence.
func NewAirtelPromptLoanRunner(deps AirtelPromptLoanDriverDeps) (*mgpoller.Runner[*models.Loan], error) {
	driver, err := NewAirtelPromptLoanDriver(deps)
	if err != nil {
		return nil, err
	}
	interval := deps.Config.PollInterval
	if interval <= 0 {
		interval = time.Minute
	}
	return mgpoller.NewRunner[*models.Loan](mgpoller.RunnerDeps[*models.Loan]{
		Direction: "airtel-prompt-loans",
		Interval:  interval,
		MaxBatch:  100,
		Fetcher: mgpoller.FetchFunc[*models.Loan](func(ctx context.Context, limit int) ([]*models.Loan, error) {
			return deps.Repo.GetDueAirtelRepayments(ctx, limit)
		}),
		Driver: driver,
		Logger: deps.Logger,
	}), nil
}
