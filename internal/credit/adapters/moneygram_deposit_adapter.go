package adapters

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/samber/oops"

	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	txmodels "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/stellaranchor"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mgpoller"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"
	"github.com/Shamba-Records-Limited/microvault/pkg/utils"
)

// Compile-time checks.
var (
	_ mgpoller.RepaymentFetcher  = (*MoneyGramDepositAdapter)(nil)
	_ mgpoller.RepaymentRecorder = (*MoneyGramDepositAdapter)(nil)
	_ mgpoller.VaultRepayer      = (*MoneyGramDepositAdapter)(nil)
)

// errDomain is the oops domain for the borrower repayment cash-in rail.
const errDomain = pkgErrors.DomainRepaymentCashIn

// adapterErr starts an error builder scoped to one loan. Loan and amount go in
// as attributes rather than into the message text, so APM tools group every
// occurrence of a failure together instead of once per loan.
func adapterErr(op, loanID string) oops.OopsErrorBuilder {
	return oops.
		In(errDomain).
		Tags("moneygram", "deposit").
		With(pkgErrors.AttrOperation, op).
		With(pkgErrors.AttrLoanID, loanID)
}

// MoneyGramDepositAdapter is the persistence and on-chain half of the borrower
// repayment cash-in rail. mgpoller owns the state machine; this supplies the
// loan projection, the state writes, and the vault leg.
type MoneyGramDepositAdapter struct {
	repo       repository.LoanRepository
	loanSvc    loan.Service
	stellarSvc stellar.Service
	txnSvc     transaction.Service
	logger     *slog.Logger
}

// DepositAdapterDeps are the collaborators the adapter needs. Repo, LoanSvc and
// StellarSvc are required; TxnSvc and Logger may be nil.
type DepositAdapterDeps struct {
	Repo       repository.LoanRepository
	LoanSvc    loan.Service
	StellarSvc stellar.Service
	TxnSvc     transaction.Service
	Logger     *slog.Logger
}

// dep pairs a dependency's name with whether it is absent, so the constructor
// validates the whole set in one pass instead of a ladder of if-statements.
type dep struct {
	name    string
	missing bool
}

// NewMoneyGramDepositAdapter builds the adapter.
func NewMoneyGramDepositAdapter(deps DepositAdapterDeps) (*MoneyGramDepositAdapter, error) {
	missing, found := lo.Find([]dep{
		{"loan_repository", deps.Repo == nil},
		{"loan_service", deps.LoanSvc == nil},
		{"stellar_service", deps.StellarSvc == nil},
	}, func(d dep) bool { return d.missing })
	if found {
		return nil, oops.
			In(errDomain).
			Code(pkgErrors.CodeMissingDependency).
			With(pkgErrors.AttrDependency, missing.name).
			Errorf("required dependency is missing")
	}

	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &MoneyGramDepositAdapter{
		repo:       deps.Repo,
		loanSvc:    deps.LoanSvc,
		stellarSvc: deps.StellarSvc,
		txnSvc:     deps.TxnSvc,
		logger:     logger.With("component", "mgdeposit_adapter"),
	}, nil
}

// GetDueRepayments returns the repayments the deposit driver should evaluate.
func (a *MoneyGramDepositAdapter) GetDueRepayments(ctx context.Context, limit int) ([]mgpoller.RepaymentRecord, error) {
	loans, err := a.repo.GetDueRepayments(ctx, limit)
	if err != nil {
		return nil, err
	}
	// FilterMap rather than a loop: nil rows are skipped and the rest projected
	// in one pass, and the result is never nil.
	return lo.FilterMap(loans, func(l *models.Loan, _ int) (mgpoller.RepaymentRecord, bool) {
		if l == nil {
			return mgpoller.RepaymentRecord{}, false
		}
		return projectRepaymentRecord(l), true
	}), nil
}

// RecordDepositUpdate persists what a polled deposit transaction tells us.
func (a *MoneyGramDepositAdapter) RecordDepositUpdate(ctx context.Context, loanID string, tx *stellaranchor.Transaction) error {
	if tx == nil {
		return adapterErr("record_deposit_update", loanID).Code(pkgErrors.CodeNilTransaction).Errorf("polled deposit transaction was nil")
	}

	deadline, hasDeadline := parseAnchorDeadline(tx.UserActionRequiredBy)
	moreInfoURL := strings.TrimSpace(tx.MoreInfoURL)
	if !hasDeadline && moreInfoURL == "" {
		return nil
	}

	loanRow, err := a.repo.GetByID(ctx, loanID)
	if err != nil {
		return adapterErr("record_deposit_update", loanID).
			Code(pkgErrors.CodeLoanLoadFailed).
			Wrapf(err, "could not load the loan")
	}

	var req loan.UpdateLoanRequest
	changed := false

	// MoneyGram's own deadline replaces the one guessed at initiation. It is
	// the binding one — the deposit lapses on user_action_required_by whatever
	// REPAYMENT_WINDOW says, and the sandbox reported roughly 24 hours against
	// a 96-hour default.
	//
	// Written every tick rather than once: MoneyGram may extend or shorten it,
	// and the last value seen is the one to act on. The write is skipped when
	// it already matches, so an unchanged deadline costs nothing.
	if hasDeadline && (loanRow.RepaymentExpiresAt == nil || !loanRow.RepaymentExpiresAt.Equal(deadline)) {
		req.RepaymentExpiresAt = &deadline
		changed = true
	}

	// The transaction page is the only artifact the borrower can act on before
	// they have paid — external_transaction_id does not exist until after. The
	// short code is minted once and only once: a second would leave the first
	// dangling in an SMS already on a handset.
	if moreInfoURL != "" && (loanRow.RampMoreInfoURL == nil || *loanRow.RampMoreInfoURL != moreInfoURL) {
		req.RampMoreInfoURL = &moreInfoURL
		changed = true
	}
	if moreInfoURL != "" && (loanRow.RampMoreInfoShortCode == nil || *loanRow.RampMoreInfoShortCode == "") {
		code, codeErr := newShortCode()
		if codeErr != nil {
			a.logger.Warn("more-info short-code generation failed",
				"loan_id", loanID, "error", codeErr)
		} else {
			req.RampMoreInfoShortCode = &code
			changed = true
		}
	}

	if !changed {
		return nil
	}

	if _, err := a.loanSvc.Update(ctx, loanID, req); err != nil {
		return adapterErr("record_deposit_update", loanID).
			Code(pkgErrors.CodeStateWriteFailed).
			With("user_action_required_by", tx.UserActionRequiredBy).
			Wrapf(err, "could not record the deposit update")
	}

	if req.RepaymentExpiresAt != nil {
		a.logger.Info("repayment deadline set from the anchor",
			"loan_id", loanID, "expires_at", deadline.Format(time.RFC3339))
	}
	if req.RampMoreInfoShortCode != nil {
		a.logger.Info("minted more-info short code for repayment",
			"loan_id", loanID, "short_code", *req.RampMoreInfoShortCode)
	}
	return nil
}

// parseAnchorDeadline reads a SEP-24 timestamp. Anything unparseable is
// ignored rather than guessed at: a misread deadline would expire a live
// deposit while the borrower is still on their way to an agent.
func parseAnchorDeadline(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// MarkFundsReceived records that the borrower's cash reached the treasury.
func (a *MoneyGramDepositAdapter) MarkFundsReceived(ctx context.Context, loanID string, tx *stellaranchor.Transaction) error {
	if tx == nil {
		return adapterErr("mark_funds_received", loanID).Code(pkgErrors.CodeNilTransaction).Errorf("polled deposit transaction was nil")
	}

	status := models.LoanRepaymentStatusFundsReceived
	if _, err := a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RepaymentStatus: &status,
	}); err != nil {
		return adapterErr("mark_funds_received", loanID).Code(pkgErrors.CodeStateWriteFailed).Wrapf(err, "could not record funds received")
	}

	a.recordCashInTransactions(ctx, loanID, tx)
	return nil
}

// MarkSettled records the confirmed vault leg and closes the loan.
func (a *MoneyGramDepositAdapter) MarkSettled(ctx context.Context, loanID, vaultTxHash string) error {
	loanRow, err := a.repo.GetByID(ctx, loanID)
	if err != nil {
		return adapterErr("mark_settled", loanID).Code(pkgErrors.CodeLoanLoadFailed).Wrapf(err, "could not load loan")
	}

	now := time.Now()
	loanRow.RepaymentStatus = models.LoanRepaymentStatusSettled
	loanRow.RepaymentVaultTxHash = &vaultTxHash
	loanRow.RepaymentNextPollAt = nil
	loanRow.Status = models.LoanStatusRepaid
	loanRow.RepaidAt = &now

	if err := a.repo.Update(ctx, loanRow); err != nil {
		return adapterErr("mark_settled", loanID).Code(pkgErrors.CodeStateWriteFailed).With("vault_tx_hash", vaultTxHash).Wrapf(err, "could not record settlement")
	}

	a.recordVaultRepayTransaction(ctx, loanRow, vaultTxHash)
	return nil
}

// MarkExpired releases the quote lock after the window elapsed.
func (a *MoneyGramDepositAdapter) MarkExpired(ctx context.Context, loanID string) error {
	return a.closeRepayment(ctx, loanID, models.LoanRepaymentStatusExpired)
}

// MarkFailed ends the rail before any funds moved.
func (a *MoneyGramDepositAdapter) MarkFailed(ctx context.Context, loanID, reason string) error {
	a.logger.Info("repayment rail failed", "loan_id", loanID, "reason", reason)
	return a.closeRepayment(ctx, loanID, models.LoanRepaymentStatusFailed)
}

// closeRepayment writes a terminal repayment status and stops the polling
// schedule. Uses the repository directly because the partial-update path
// cannot write a NULL — a nil pointer there means "leave unchanged", and
// leaving next_poll_at set would keep a finished repayment in the due set.
func (a *MoneyGramDepositAdapter) closeRepayment(ctx context.Context, loanID, status string) error {
	loanRow, err := a.repo.GetByID(ctx, loanID)
	if err != nil {
		return adapterErr("close_repayment", loanID).Code(pkgErrors.CodeLoanLoadFailed).With("target_status", status).Wrapf(err, "could not load loan")
	}
	loanRow.RepaymentStatus = status
	loanRow.RepaymentNextPollAt = nil
	if err := a.repo.Update(ctx, loanRow); err != nil {
		return adapterErr("close_repayment", loanID).Code(pkgErrors.CodeStateWriteFailed).With("target_status", status).Wrapf(err, "could not write terminal repayment status")
	}
	return nil
}

// MarkReminderSent stamps the pre-expiry reminder before it is sent.
func (a *MoneyGramDepositAdapter) MarkReminderSent(ctx context.Context, loanID string) error {
	now := time.Now()
	if _, err := a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RepaymentReminderSentAt: &now,
	}); err != nil {
		return adapterErr("mark_reminder_sent", loanID).Code(pkgErrors.CodeStateWriteFailed).Wrapf(err, "could not stamp reminder as sent")
	}
	return nil
}

// RecordVaultAttempt persists the failed-vault-leg count.
//
// Written on every failure rather than only at the ceiling: the count is what
// survives a restart, and without it a process bounce would reset a loan that
// has failed fifty times back to zero and the escalation would never fire.
func (a *MoneyGramDepositAdapter) RecordVaultAttempt(ctx context.Context, loanID string, attempts int) error {
	if _, err := a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RepaymentVaultAttempts: &attempts,
	}); err != nil {
		return adapterErr("record_vault_attempt", loanID).
			Code(pkgErrors.CodeStateWriteFailed).
			With("attempts", attempts).
			Wrapf(err, "could not record the vault repay attempt")
	}
	return nil
}

// MarkReferenceSent stamps the deposit-reference SMS before it is sent.
func (a *MoneyGramDepositAdapter) MarkReferenceSent(ctx context.Context, loanID string) error {
	now := time.Now()
	if _, err := a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RepaymentReferenceSentAt: &now,
	}); err != nil {
		return adapterErr("mark_reference_sent", loanID).
			Code(pkgErrors.CodeStateWriteFailed).
			Wrapf(err, "could not stamp the deposit reference as sent")
	}
	return nil
}

// ScheduleNextPoll sets when this repayment is next looked at.
func (a *MoneyGramDepositAdapter) ScheduleNextPoll(ctx context.Context, loanID string, at time.Time) error {
	if _, err := a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RepaymentNextPollAt: &at,
	}); err != nil {
		return adapterErr("schedule_next_poll", loanID).Code(pkgErrors.CodeStateWriteFailed).With("next_poll_at", at).Wrapf(err, "could not set the next poll time")
	}
	return nil
}

// RepayForBorrower settles the on-chain leg, attributed to the borrower's
// child account.
func (a *MoneyGramDepositAdapter) RepayForBorrower(ctx context.Context, loanID, borrowerAddress string, amountStroops int64) (string, error) {
	if borrowerAddress == "" {
		return "", adapterErr("repay_for_borrower", loanID).Code(pkgErrors.CodeMissingBorrowerAddr).Errorf("loan has no borrower address to attribute the repayment to")
	}
	if amountStroops <= 0 {
		return "", adapterErr("repay_for_borrower", loanID).Code(pkgErrors.CodeInvalidAmount).With(pkgErrors.AttrAmountStroops, amountStroops).Errorf("refusing to repay a non-positive amount")
	}

	resp, err := a.stellarSvc.RepayForVault(ctx, stellar.RepayForRequest{
		BorrowerAddress: borrowerAddress,
		Amount:          amountStroops,
	})
	if err != nil {
		return "", adapterErr("repay_for_borrower", loanID).
			Code(pkgErrors.CodeVaultRepayFailed).
			With(pkgErrors.AttrAmountStroops, amountStroops).
			With(pkgErrors.AttrBorrower, borrowerAddress).
			Wrapf(err, "vault repay leg failed")
	}
	return resp.TxHash, nil
}

// recordCashInTransactions writes the two inbound legs of a repayment: the
// borrower's cash at the counter and the anchor's USDC to the treasury.
func (a *MoneyGramDepositAdapter) recordCashInTransactions(ctx context.Context, loanID string, tx *stellaranchor.Transaction) {
	if a.txnSvc == nil {
		return
	}
	loanRow, err := a.repo.GetByID(ctx, loanID)
	if err != nil {
		a.logger.Warn("could not load loan to record cash-in transactions",
			"loan_id", loanID, "error", err)
		return
	}

	provider := "moneygram"
	payoff := int64(0)
	if loanRow.RepaymentPayoffStroops != nil {
		payoff = *loanRow.RepaymentPayoffStroops
	}

	// Off-chain leg: the cash handed over at the agent counter. amount_in is
	// the local currency the borrower paid.
	cashDesc := "Borrower cash repayment paid in at a MoneyGram agent"
	cashAmount, currency := payoff, "USDC"
	if cents, ok := decimalToCents(tx.AmountIn); ok && cents > 0 {
		cashAmount = cents
		currency = strings.TrimPrefix(strings.TrimSpace(tx.AmountInAsset), "iso4217:")
	}
	if currency == "" {
		currency = "USDC"
	}
	if txnResp, err := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
		UserID:           &loanRow.UserID,
		AccountID:        &loanRow.AccountID,
		LoanID:           &loanID,
		TxType:           txmodels.TxTypeLoanRepayment,
		Amount:           cashAmount,
		Asset:            currency,
		ExternalID:       loanRow.RepaymentMGTxID,
		ExternalProvider: &provider,
		Description:      &cashDesc,
	}); err != nil {
		a.logger.Warn("failed to record loan repayment transaction",
			"loan_id", loanID, "error", err)
	} else if txnResp != nil {
		a.settleTransaction(ctx, txnResp.ID, "loan_repayment", loanID, txmodels.TxStatusSuccess)
	}

	// On-chain leg: the anchor crediting the treasury in USDC.
	//
	// The amount is what MoneyGram says it credited, not what we quoted. SEP-24
	// defines amount_out as net of fees, so on a fee-bearing corridor the two
	// differ — and a ledger row carrying the quote instead of the credit would
	// make reconciliation agree with itself while disagreeing with the chain.
	// Falls back to the payoff only when amount_out is unreadable.
	depositDesc := "USDC credited to treasury by MoneyGram for a borrower repayment"
	creditedStroops := payoff
	if stroops, ok := utils.ParseDecimalStroops(tx.AmountOut); ok && stroops > 0 {
		creditedStroops = stroops
	}
	var stellarHash *string
	if h := strings.TrimSpace(tx.StellarTransactionID); h != "" {
		stellarHash = &h
	}
	if txnResp, err := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
		UserID:           &loanRow.UserID,
		AccountID:        &loanRow.AccountID,
		LoanID:           &loanID,
		TxType:           txmodels.TxTypeAnchorDeposit,
		Amount:           creditedStroops,
		Asset:            "USDC",
		StellarTxHash:    stellarHash,
		ExternalID:       loanRow.RepaymentMGTxID,
		ExternalProvider: &provider,
		Description:      &depositDesc,
	}); err != nil {
		a.logger.Warn("failed to record anchor deposit transaction",
			"loan_id", loanID, "error", err)
	} else if txnResp != nil {
		a.settleTransaction(ctx, txnResp.ID, "anchor_deposit", loanID, txmodels.TxStatusSuccess)
	}
}

// recordVaultRepayTransaction writes the third leg: treasury to vault.
func (a *MoneyGramDepositAdapter) recordVaultRepayTransaction(ctx context.Context, loanRow *models.Loan, txHash string) {
	if a.txnSvc == nil {
		return
	}
	amount := int64(0)
	if loanRow.RepaymentPayoffStroops != nil {
		amount = *loanRow.RepaymentPayoffStroops
	}
	desc := "USDC returned to the vault for a borrower repayment, attributed via repay_for"
	provider := "moneygram"

	txnResp, err := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
		UserID:           &loanRow.UserID,
		AccountID:        &loanRow.AccountID,
		LoanID:           &loanRow.ID,
		TxType:           txmodels.TxTypeVaultRepay,
		Amount:           amount,
		Asset:            "USDC",
		StellarTxHash:    &txHash,
		ExternalID:       loanRow.RepaymentMGTxID,
		ExternalProvider: &provider,
		Description:      &desc,
	})
	if err != nil {
		a.logger.Warn("failed to record vault repay transaction",
			"loan_id", loanRow.ID, "error", err)
		return
	}
	if txnResp != nil {
		a.settleTransaction(ctx, txnResp.ID, "vault_repay", loanRow.ID, txmodels.TxStatusSuccess)
	}
}

// settleTransaction advances a freshly created row to its final status. Shares
// the withdrawal adapter's helper of the same shape; kept as a thin method here
// so this file does not reach into the other adapter's receiver.
func (a *MoneyGramDepositAdapter) settleTransaction(ctx context.Context, txnID, kind, loanID, final string) {
	steps := []string{txmodels.TxStatusSubmitted}
	if final == txmodels.TxStatusSuccess {
		steps = append(steps, txmodels.TxStatusSuccess)
	}
	for _, step := range steps {
		status := step
		if _, err := a.txnSvc.Update(ctx, txnID, transaction.UpdateTransactionRequest{
			Status: &status,
		}); err != nil {
			a.logger.Warn("failed to advance transaction status",
				"kind", kind, "loan_id", loanID, "transaction_id", txnID,
				"to", status, "error", err)
			return
		}
	}
}

// projectRepaymentRecord maps a loan row into the deposit driver's projection.
func projectRepaymentRecord(l *models.Loan) mgpoller.RepaymentRecord {
	rec := mgpoller.RepaymentRecord{
		LoanID:          l.ID,
		UserID:          l.UserID,
		RepaymentStatus: l.RepaymentStatus,
		ReminderSent:    l.RepaymentReminderSentAt != nil,
		VaultAttempts:   l.RepaymentVaultAttempts,
		ReferenceSent:   l.RepaymentReferenceSentAt != nil,
	}
	if l.RampSequenceID != nil {
		rec.SequenceID = *l.RampSequenceID
	}
	if l.RepaymentMGTxID != nil {
		rec.MoneyGramTxID = *l.RepaymentMGTxID
	}
	if l.RampChildAccountIndex != nil {
		// Stored as int64 to fit the column, fits a uint32 by construction.
		rec.ChildAccountIndex = uint32(*l.RampChildAccountIndex)
	}
	if l.RepaymentPayoffStroops != nil {
		rec.PayoffStroops = *l.RepaymentPayoffStroops
	}
	if l.RepaymentExpiresAt != nil {
		rec.ExpiresAt = *l.RepaymentExpiresAt
	}
	if l.User != nil {
		rec.PhoneNumber = l.User.MobileNumber
	}
	// The child account's own address is what repay_for attributes the
	// repayment to on-chain.
	if l.Account != nil {
		rec.BorrowerAddress = l.Account.PublicKey
	}
	return rec
}
