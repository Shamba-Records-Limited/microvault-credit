package adapters

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"

	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	"github.com/Shamba-Records-Limited/microvault/pkg/logging"
	"github.com/Shamba-Records-Limited/microvault/pkg/telemetry"

	"github.com/samber/lo"

	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	coremodels "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
)

// paybillSweepBatchSize bounds one tick's conversions. Generous for paybill's
// expected volume; a backlog beyond this just drains over more ticks rather
// than blocking anything.
const paybillSweepBatchSize = 100

// paybillFXCurrency is the only currency mpesa_transactions.amount_kes is
// ever denominated in — the rail is Kenyan M-Pesa.
const paybillFXCurrency = "KES"

// stroopsPerUSDC mirrors the convention used throughout this codebase: 1
// USDC = 10_000_000 stroops.
const stroopsPerUSDC = 1e7

// repaymentQuoter is the one LoanServiceAdapter method the driver needs —
// narrow enough that a fake can stand in for it in tests without a real
// Stellar RPC client. GetRepaymentQuote's AmountUSDCStroops is derived purely
// from the borrow index, no FX involved, which is why it is reused for the
// payoff lock but not for pricing an individual paybill payment (see
// fxRateForLoan).
type repaymentQuoter interface {
	GetRepaymentQuote(ctx context.Context, loanID string) (*ussd.RepaymentQuote, error)
}

// MpesaPaybillRepaymentDriver converts confirmed, loan-attributed paybill
// payments into repayment progress. Unlike the STK driver, nothing here is
// "initiated" first — a paybill is always open, so a borrower can pay it
// without any prior app interaction, and the first confirmed payment this
// driver sees for a loan is what opens its repayment.
//
// mpesa_transactions.applied_stroops (not a running total on loans) is the
// source of truth for how much a loan has received; see the migration's
// comment on why.
type MpesaPaybillRepaymentDriver struct {
	mpesaRepo corerepository.MpesaTransactionRepository
	repo      repository.LoanRepository
	quoter    repaymentQuoter
	offRamps  *offramp.Registry
	notifier  stkRepaymentNotifier
	logger    *slog.Logger
	interval  time.Duration
}

// MpesaPaybillRepaymentDriverDeps are the collaborators the driver needs; all
// required except Notifier and Logger.
type MpesaPaybillRepaymentDriverDeps struct {
	MpesaRepo corerepository.MpesaTransactionRepository
	Repo      repository.LoanRepository
	Quoter    repaymentQuoter
	OffRamps  *offramp.Registry
	Notifier  stkRepaymentNotifier
	Interval  time.Duration
	Logger    *slog.Logger
}

// NewMpesaPaybillRepaymentDriver builds the driver.
func NewMpesaPaybillRepaymentDriver(deps MpesaPaybillRepaymentDriverDeps) (*MpesaPaybillRepaymentDriver, error) {
	if deps.MpesaRepo == nil || deps.Repo == nil || deps.Quoter == nil || deps.OffRamps == nil {
		return nil, adapterErr("new_mpesa_paybill_driver", "").
			Errorf("mpesa repo, loan repo, quoter and offramp registry are all required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &MpesaPaybillRepaymentDriver{
		mpesaRepo: deps.MpesaRepo,
		repo:      deps.Repo,
		quoter:    deps.Quoter,
		offRamps:  deps.OffRamps,
		notifier:  deps.Notifier,
		logger:    logger.With("component", "mpesa_paybill_repayment_driver"),
		interval:  deps.Interval,
	}, nil
}

// Start runs the sweep until ctx is cancelled — the same
// tick-immediately-then-on-interval shape as every other ticker in this
// codebase.
func (d *MpesaPaybillRepaymentDriver) Start(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	d.logger.InfoContext(ctx, "starting", "interval", d.interval)
	d.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			d.logger.InfoContext(ctx, "shutting down")
			return
		case <-ticker.C:
			d.tick(ctx)
		}
	}
}

// tick converts one batch of unapplied observations and, for every loan that
// batch touched, recomputes its progress and advances repayment_status.
func (d *MpesaPaybillRepaymentDriver) tick(ctx context.Context) {
	txs, err := d.mpesaRepo.ListUnappliedConfirmed(ctx, paybillSweepBatchSize)
	if err != nil {
		d.logger.ErrorContext(ctx, "could not list unapplied paybill payments", "error", err)
		return
	}

	touched := make(map[string]struct{}, len(txs))
	for _, tx := range txs {
		if tx.LoanID == nil {
			continue
		}
		if d.traceApply(ctx, tx) {
			touched[*tx.LoanID] = struct{}{}
		}
	}

	for loanID := range touched {
		d.recompute(ctx, loanID)
	}
}

// paybillEligible reports whether a loan may currently accrue paybill
// repayment progress: disbursed, and not mid-repayment on a different rail.
// A loan already partway through an M-Pesa paybill repayment (or with no
// repayment open at all) is eligible; one MoneyGram or STK currently owns is
// not — a stray paybill payment against those must not silently reassign
// the rail.
func paybillEligible(l *models.Loan) bool {
	if l.Status != models.LoanStatusDisbursed {
		return false
	}
	switch l.RepaymentStatus {
	case models.LoanRepaymentStatusNone, models.LoanRepaymentStatusPartialFundsReceived:
		return true
	case models.LoanRepaymentStatusInitiated, models.LoanRepaymentStatusFundsReceived:
		return l.RepaymentProvider == models.LoanRepaymentProviderMpesa
	default:
		return false
	}
}

func (d *MpesaPaybillRepaymentDriver) traceApply(ctx context.Context, tx *coremodels.MpesaTransaction) bool {
	ctx = logging.With(ctx, slog.String(pkgErrors.AttrLoanID, *tx.LoanID))
	ctx, span := telemetry.StartRoot(ctx, "mpesa.paybill_apply",
		attribute.String(pkgErrors.AttrLoanID, *tx.LoanID), attribute.String("mpesa_transaction_id", tx.ID))
	defer span.End()
	return d.applyOne(ctx, tx)
}

// applyOne converts one observation to stroops and records it on the row.
// Reports whether the row was applied — false leaves it unapplied, retried
// on the next tick, for any failure that might resolve on its own (FX
// unavailable, the loan row failing to load).
func (d *MpesaPaybillRepaymentDriver) applyOne(ctx context.Context, tx *coremodels.MpesaTransaction) bool {
	loanID := *tx.LoanID
	logger := d.logger.With("loan_id", loanID, "trans_id", tx.TransID)

	loanRow, err := d.repo.GetByID(ctx, loanID)
	if err != nil {
		logger.ErrorContext(ctx, "could not load loan for a paybill payment, will retry next tick", "error", err)
		return false
	}
	if !paybillEligible(loanRow) {
		logger.WarnContext(ctx, "paybill payment for a loan that cannot currently accrue mpesa repayment progress",
			"loan_status", loanRow.Status, "repayment_status", loanRow.RepaymentStatus, "repayment_provider", loanRow.RepaymentProvider)
		return false
	}

	if loanRow.RepaymentPayoffStroops == nil {
		if err := d.lockPayoff(ctx, loanRow); err != nil {
			logger.WarnContext(ctx, "could not lock a payoff for a paybill payment, will retry next tick", "error", err)
			return false
		}
	}

	rate, rateSource, err := d.fxRateForLoan(ctx, loanRow)
	if err != nil {
		logger.WarnContext(ctx, "no FX rate available for a paybill payment, will retry next tick", "error", err)
		return false
	}

	amountUSD := (float64(tx.AmountKes) / 100.0) / rate
	stroops := int64(amountUSD * stroopsPerUSDC)
	if stroops <= 0 {
		logger.ErrorContext(ctx, "paybill payment converted to a non-positive stroops figure, dropping it from the sweep",
			"amount_kes", tx.AmountKes, "fx_rate", rate, "fx_source", rateSource)
		// Not retried: a non-positive conversion will never become positive,
		// and leaving it in the unapplied queue forever would starve every
		// other loan's payments behind it once the batch fills with it.
		if err := d.mpesaRepo.SetAppliedStroops(ctx, tx.ID, 0); err != nil {
			logger.ErrorContext(ctx, "could not mark the non-positive payment as applied", "error", err)
			return false
		}
		return true
	}

	if err := d.mpesaRepo.SetAppliedStroops(ctx, tx.ID, stroops); err != nil {
		logger.ErrorContext(ctx, "could not record the converted stroops figure, will retry next tick", "error", err)
		return false
	}

	logger.InfoContext(ctx, "paybill payment converted",
		"amount_kes", tx.AmountKes, "stroops", stroops, "fx_rate", rate, "fx_source", rateSource)
	return true
}

// lockPayoff quote-locks the payoff a walk-up-and-pay paybill loan has no
// other moment to lock it at — see MpesaPaybillRepaymentDriver's doc
// comment. Mirrors what MpesaCollectionAdapter.Prompt does for STK, minus
// the STK-specific fields.
func (d *MpesaPaybillRepaymentDriver) lockPayoff(ctx context.Context, l *models.Loan) error {
	quote, err := d.quoter.GetRepaymentQuote(ctx, l.ID)
	if err != nil {
		return err
	}
	l.RepaymentPayoffStroops = lo.ToPtr(quote.AmountUSDCStroops)
	now := time.Now()
	l.RepaymentLockedAt = &now
	if l.RepaymentProvider == "" {
		l.RepaymentProvider = models.LoanRepaymentProviderMpesa
	}
	return d.repo.Update(ctx, l)
}

// fxRateForLoan resolves an FX sell rate per the disbursement provider's own
// quote, falling back to YellowCard's if the disbursing provider has none —
// see offrampSellRate, shared with RepaymentNotifierAdapter's SMS pricing.
func (d *MpesaPaybillRepaymentDriver) fxRateForLoan(ctx context.Context, l *models.Loan) (float64, string, error) {
	return offrampSellRate(ctx, d.offRamps, l.RampProvider, paybillFXCurrency)
}

// recompute totals every applied observation for a loan and advances its
// repayment_status. Safe to call repeatedly: the sum it writes is a pure
// aggregate of mpesa_transactions.applied_stroops, never a value that could
// itself drift from what it is summing.
func (d *MpesaPaybillRepaymentDriver) recompute(ctx context.Context, loanID string) {
	logger := d.logger.With("loan_id", loanID)

	total, err := d.mpesaRepo.SumAppliedStroopsByLoan(ctx, loanID)
	if err != nil {
		logger.ErrorContext(ctx, "could not total a loan's applied paybill payments", "error", err)
		return
	}

	loanRow, err := d.repo.GetByID(ctx, loanID)
	if err != nil {
		logger.ErrorContext(ctx, "could not load loan to record paybill progress", "error", err)
		return
	}
	if loanRow.RepaymentPayoffStroops == nil {
		// Every row that reached this point locked a payoff before being
		// applied (see applyOne), so a nil payoff here means the loan moved
		// out of repayment entirely (e.g. reassigned) between applyOne and
		// this call — leave it alone rather than guessing a status.
		logger.WarnContext(ctx, "loan has applied paybill payments but no locked payoff, leaving its status untouched")
		return
	}

	wasFundsReceived := loanRow.RepaymentStatus == models.LoanRepaymentStatusFundsReceived ||
		loanRow.RepaymentStatus == models.LoanRepaymentStatusSettled
	loanRow.RepaymentReceivedStroops = lo.ToPtr(total)
	if total >= *loanRow.RepaymentPayoffStroops {
		loanRow.RepaymentStatus = models.LoanRepaymentStatusFundsReceived
	} else {
		loanRow.RepaymentStatus = models.LoanRepaymentStatusPartialFundsReceived
	}

	if err := d.repo.Update(ctx, loanRow); err != nil {
		logger.ErrorContext(ctx, "could not record paybill repayment progress", "error", err)
		return
	}

	logger.InfoContext(ctx, "paybill repayment progress recorded",
		"received_stroops", total, "payoff_stroops", *loanRow.RepaymentPayoffStroops, "status", loanRow.RepaymentStatus)

	if !wasFundsReceived && loanRow.RepaymentStatus == models.LoanRepaymentStatusFundsReceived {
		recordRepayment(ctx, railMpesaPaybill, models.LoanRepaymentStatusFundsReceived)
		d.notify(ctx, loanID)
	}
}

// notify tells the borrower their payment landed, mirroring
// MpesaSTKLoanDriver.notify exactly — best-effort, never blocks the state
// write above.
func (d *MpesaPaybillRepaymentDriver) notify(ctx context.Context, loanID string) {
	if d.notifier == nil {
		d.logger.WarnContext(ctx, "no repayment notifier configured, message not sent", "loan_id", loanID)
		return
	}
	if err := d.notifier.NotifyRepaymentReceivedAmount(ctx, loanID, 0); err != nil {
		d.logger.WarnContext(ctx, "failed to send repayment received notification", "loan_id", loanID, "error", err)
	}
}
