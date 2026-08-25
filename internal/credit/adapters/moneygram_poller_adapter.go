package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	txmodels "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/stellaranchor"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mgpoller"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"

	"github.com/samber/oops"

	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
)

// Compile-time checks.
var (
	_ mgpoller.LoanFetcher  = (*MoneyGramPollerAdapter)(nil)
	_ mgpoller.LoanRecorder = (*MoneyGramPollerAdapter)(nil)
)

// pollerAdapterErr starts an error builder for the withdrawal poller's
// persistence half.
func pollerAdapterErr(op string) oops.OopsErrorBuilder {
	return oops.In(pkgErrors.DomainMoneyGramPoller).
		Tags("withdrawal").
		With(pkgErrors.AttrDirection, "withdrawal").
		With(pkgErrors.AttrOperation, op)
}

// MoneyGramPollerAdapter bridges microvault-credit's loan repository to the
// generic mgpoller in microvault. mgpoller defines the state machine and
// HTTP loop; this adapter supplies the persistence half — projecting loan
// rows into mgpoller.LoanRecord and writing the per-tick deltas back to the
// loan via the loan service's typed update path.
type MoneyGramPollerAdapter struct {
	repo    repository.LoanRepository
	loanSvc loan.Service
	txnSvc  transaction.Service
	logger  *slog.Logger
}

// NewMoneyGramPollerAdapter builds the adapter. repo and loanSvc are
// required; txnSvc and logger may be nil.
func NewMoneyGramPollerAdapter(
	repo repository.LoanRepository,
	loanSvc loan.Service,
	txnSvc transaction.Service,
	logger *slog.Logger,
) (*MoneyGramPollerAdapter, error) {
	if repo == nil {
		return nil, pollerAdapterErr("new").With(pkgErrors.AttrDependency, "loan_repository").
			Code(pkgErrors.CodeMissingDependency).Errorf("required dependency is missing")
	}
	if loanSvc == nil {
		return nil, pollerAdapterErr("new").With(pkgErrors.AttrDependency, "loan_service").
			Code(pkgErrors.CodeMissingDependency).Errorf("required dependency is missing")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &MoneyGramPollerAdapter{
		repo:    repo,
		loanSvc: loanSvc,
		txnSvc:  txnSvc,
		logger:  logger.With("component", "mgpoller_adapter"),
	}, nil
}

// GetActiveMoneyGramLoans returns the loans the mgpoller should evaluate
// this tick: ramp_provider="moneygram" with non-terminal disbursement_status.
func (a *MoneyGramPollerAdapter) GetActiveMoneyGramLoans(ctx context.Context, limit int) ([]mgpoller.LoanRecord, error) {
	loans, err := a.repo.GetActiveMoneyGramLoans(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]mgpoller.LoanRecord, 0, len(loans))
	for _, l := range loans {
		if l == nil {
			continue
		}
		out = append(out, projectLoanRecord(l))
	}
	return out, nil
}

// RecordTransactionUpdate persists the SEP-24 transaction fields onto the
// loan: amount_out + currency, fee, MG external reference, MoreInfoURL, and
// the withdraw memo MG hands back (needed by the Stellar ingest worker for
// refund matching). All fields are optional in the SEP-24 response — we
// only set those that arrived non-empty.
func (a *MoneyGramPollerAdapter) RecordTransactionUpdate(ctx context.Context, loanID string, tx *stellaranchor.Transaction) error {
	if tx == nil {
		return pollerAdapterErr("record_transaction_update").With(pkgErrors.AttrLoanID, loanID).
			Code(pkgErrors.CodeNilTransaction).Errorf("polled transaction was nil")
	}

	req := loan.UpdateLoanRequest{}
	any := false

	var grossCents int64
	var grossKnown bool
	if v := strings.TrimSpace(tx.AmountOut); v != "" {
		cents, ok := decimalToCents(v)
		if ok {
			req.RampFiatAmount = &cents
			grossCents = cents
			grossKnown = true
			any = true
		}
	}
	if v := strings.TrimPrefix(strings.TrimSpace(tx.AmountOutAsset), "iso4217:"); v != "" {
		req.RampFiatCurr = &v
		any = true
	}
	var feeCents int64
	if v := strings.TrimSpace(tx.AmountFee); v != "" {
		cents, ok := decimalToCents(v)
		if ok {
			req.ServiceFeeLocal = &cents
			feeCents = cents
			any = true
		}
	}
	// MG's SEP-24 amount_out is the value the user receives, already net of
	// amount_fee — so it equals delivered_amount_local.
	_ = feeCents
	if grossKnown {
		delivered := grossCents
		req.DeliveredAmountLocal = &delivered
	}
	if v := strings.TrimSpace(tx.ExternalTransactionID); v != "" {
		req.RampExternalRef = &v
		any = true
	}
	if v := strings.TrimSpace(tx.MoreInfoURL); v != "" {
		req.RampMoreInfoURL = &v
		any = true
		// Generated here rather than at initiate: MoneyGram only returns
		// more_info_url once the withdrawal is under way, so a code minted
		// earlier would redirect to nothing. Minted once — a second code would
		// leave the first dangling in an already-delivered SMS.
		if a.needsMoreInfoShortCode(ctx, loanID) {
			if code, err := newShortCode(); err != nil {
				a.logger.Warn("more-info short-code generation failed",
					"loan_id", loanID, "error", err)
			} else {
				req.RampMoreInfoShortCode = &code
			}
		}
	}
	if v := strings.TrimSpace(tx.WithdrawMemo); v != "" {
		req.RampWithdrawMemo = &v
		any = true
	}
	if v := strings.TrimSpace(tx.WithdrawMemoType); v != "" {
		req.RampWithdrawMemoType = &v
		any = true
	}

	if !any {
		return nil
	}

	// Read before writing: the guard against a duplicate off-ramp row is
	// "ramp_fiat_amount was not set yet", and the Update below sets it. The
	// row is carried forward so the write path does not re-read it.
	var pending *models.Loan
	if grossKnown && payoutLocked(tx.Status) {
		pending = a.loanAwaitingOffRampRow(ctx, loanID)
	}

	if _, err := a.loanSvc.Update(ctx, loanID, req); err != nil {
		return pollerAdapterErr("record_transaction_update").With(pkgErrors.AttrLoanID, loanID).
			Code(pkgErrors.CodeStateWriteFailed).Wrapf(err, "could not update the loan")
	}

	if pending != nil {
		a.recordOffRampTransaction(ctx, loanID, pending, grossCents, req.RampFiatCurr, req.RampExternalRef, tx.Status)
	}
	return nil
}

// payoutLocked reports whether MG has committed to a payout amount. Before
// this point amount_out may be absent or an estimate, and transaction amounts
// are immutable once written — so recording early risks a permanently wrong
// row, which is the reason the fiat leg is not written at initiate.
func payoutLocked(status stellaranchor.Status) bool {
	return status == stellaranchor.StatusPendingUserTransferComplete ||
		status == stellaranchor.StatusCompleted
}

// loanAwaitingOffRampRow returns the loan when its locked payout has not been
// recorded yet, and nil when it has — so repeated ticks do not each write an
// off-ramp row. Errs toward "already recorded": if the loan cannot be read,
// skipping the row loses an audit entry, while writing it risks duplicating a
// financial record.
func (a *MoneyGramPollerAdapter) loanAwaitingOffRampRow(ctx context.Context, loanID string) *models.Loan {
	loanRow, err := a.repo.GetByID(ctx, loanID)
	if err != nil {
		a.logger.Warn("could not load loan to check off-ramp transaction; skipping",
			"loan_id", loanID, "error", err)
		return nil
	}
	if loanRow.RampFiatAmount != nil {
		return nil
	}
	return loanRow
}

// recordOffRampTransaction writes the off-chain fiat leg of a cash-pickup
// disbursement — the counterpart to the mobile-money row written in
// recordSuccessfulInitiate.
//
// It is written here rather than at initiate because MoneyGram returns no
// locked amount from the withdraw call: the figure only exists once the user
// has completed the anchor webview and MG commits to a payout.
func (a *MoneyGramPollerAdapter) recordOffRampTransaction(ctx context.Context, loanID string, loanRow *models.Loan, amountCents int64, currency, externalRef *string, status stellaranchor.Status) {
	if a.txnSvc == nil {
		return
	}

	asset := ""
	if currency != nil {
		asset = *currency
	}
	desc := "Off-ramp via moneygram (cash_pickup)"
	provider := "moneygram"

	txnResp, err := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
		UserID:           &loanRow.UserID,
		AccountID:        &loanRow.AccountID,
		LoanID:           &loanID,
		TxType:           txmodels.TxTypeOffRamp,
		Amount:           amountCents,
		Asset:            asset,
		ExternalID:       loanRow.RampRequestID,
		ExternalProvider: &provider,
		Description:      &desc,
		// externalRef is MG's cash-pickup reference — the number the borrower
		// quotes at the counter. It appears only once MG commits to a payout,
		// so it cannot be the row's external_id, which is set at creation.
		Metadata: txMetadata(map[string]any{
			"external_transaction_id": externalRef,
			"settlement_method":       loanRow.SettlementMethod,
		}),
	})
	if err != nil {
		a.logger.Warn("failed to record off-ramp transaction",
			"loan_id", loanID, "error", err)
		return
	}

	// A locked payout has been committed by MG but not yet collected by the
	// borrower; only `completed` means the cash is actually in hand.
	final := txmodels.TxStatusSubmitted
	if status == stellaranchor.StatusCompleted {
		final = txmodels.TxStatusSuccess
	}
	if txnResp != nil {
		a.settleTransaction(ctx, txnResp.ID, "off_ramp", loanID, final)
	}

	a.logger.Info("cash-pickup off-ramp transaction recorded",
		"loan_id", loanID, "amount_cents", amountCents, "currency", asset, "status", final)
}

// RecordSendUSDC persists the treasury to MG anchor tx hash on the loan and
// writes the matching on-chain transaction row. The hash doubles as the
// idempotency marker: its presence makes HasStellarSend true, so a later tick
// refuses to pay twice even if MG is slow to echo stellar_transaction_id.
// sendPendingMarker is written to loans.ramp_stellar_tx_hash to claim a send
// before it is submitted. A real hash is 64 hex chars, so this sentinel is
// unambiguous. Its presence makes HasStellarSend true, which blocks a second
// payment if the process dies mid-send.
const sendPendingMarker = "pending"

// RecordSendAttempt claims the send before submission. See sendPendingMarker.
func (a *MoneyGramPollerAdapter) RecordSendAttempt(ctx context.Context, loanID string) error {
	marker := sendPendingMarker
	if _, err := a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RampStellarTxHash: &marker,
	}); err != nil {
		return pollerAdapterErr("record_send_attempt").With(pkgErrors.AttrLoanID, loanID).
			Code(pkgErrors.CodeStateWriteFailed).Wrapf(err, "could not claim the send attempt")
	}
	return nil
}

// ClearSendAttempt releases the claim after a payment that definitively moved
// no funds, so a later tick can retry.
func (a *MoneyGramPollerAdapter) ClearSendAttempt(ctx context.Context, loanID string) error {
	empty := ""
	if _, err := a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RampStellarTxHash: &empty,
	}); err != nil {
		return pollerAdapterErr("clear_send_attempt").With(pkgErrors.AttrLoanID, loanID).
			Code(pkgErrors.CodeStateWriteFailed).Wrapf(err, "could not release the send claim")
	}
	return nil
}

func (a *MoneyGramPollerAdapter) RecordSendUSDC(ctx context.Context, loanID string, txHash string) error {
	if txHash == "" {
		return pollerAdapterErr("record_send_usdc").With(pkgErrors.AttrLoanID, loanID).
			Code(pkgErrors.CodeIncompleteResponse).Errorf("send transaction hash is empty")
	}

	// Persisted so the poller's idempotency guard survives a slow MoneyGram
	// stellar_transaction_id echo — without this it re-sends every tick.
	if _, err := a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RampStellarTxHash: &txHash,
	}); err != nil {
		return pollerAdapterErr("record_send_usdc").With(pkgErrors.AttrLoanID, loanID).
			Code(pkgErrors.CodeStateWriteFailed).Wrapf(err, "could not record the send hash")
	}

	a.recordAnchorTransferTransaction(ctx, loanID, txHash)

	a.logger.Info("treasury to MoneyGram USDC send recorded",
		"loan_id", loanID, "tx_hash", txHash)
	return nil
}

// recordAnchorTransferTransaction writes the on-chain USDC leg of the anchor
// withdrawal to the transactions ledger.
//
// The mobile-money path records its off-ramp in recordSuccessfulInitiate, but
// that branch is YellowCard-only, so cash-pickup disbursements previously left
// no transaction row at all — the treasury payment existed solely as
// loans.ramp_stellar_tx_hash and a log line.
//
// Best effort: the USDC has already left the treasury by the time this runs,
// so a bookkeeping failure must not fail the send or trigger a re-send.
func (a *MoneyGramPollerAdapter) recordAnchorTransferTransaction(ctx context.Context, loanID, txHash string) {
	if a.txnSvc == nil {
		return
	}

	loanRow, err := a.repo.GetByID(ctx, loanID)
	if err != nil {
		a.logger.Warn("could not load loan to record anchor transfer transaction",
			"loan_id", loanID, "error", err)
		return
	}

	desc := "USDC sent from treasury to MoneyGram anchor for cash-pickup withdrawal"
	provider := "moneygram"

	txnResp, err := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
		UserID:           &loanRow.UserID,
		AccountID:        &loanRow.AccountID,
		LoanID:           &loanID,
		TxType:           txmodels.TxTypeAnchorTransfer,
		Amount:           loanRow.PrincipalAmount,
		Asset:            "USDC",
		StellarTxHash:    &txHash,
		ExternalID:       loanRow.RampRequestID,
		ExternalProvider: &provider,
		Description:      &desc,
		// The memo is how MG attributes this inbound payment to the withdrawal;
		// without it a payment landing at the anchor cannot be traced back here.
		Metadata: txMetadata(map[string]any{
			"withdraw_memo":      loanRow.RampWithdrawMemo,
			"withdraw_memo_type": loanRow.RampWithdrawMemoType,
		}),
	})
	if err != nil {
		a.logger.Warn("failed to record anchor transfer transaction",
			"loan_id", loanID, "error", err)
		return
	}

	// SendUSDC only returns a hash once the payment is confirmed on-ledger, so
	// the row is already settled and must not be left pending.
	if txnResp != nil {
		a.settleTransaction(ctx, txnResp.ID, "anchor_transfer", loanID, txmodels.TxStatusSuccess)
	}
}

// RecordRefund persists a settled MoneyGram refund. Written before the vault
// repay so a crash mid-repay still leaves a record of what came back and in
// which Stellar transaction.
func (a *MoneyGramPollerAdapter) RecordRefund(ctx context.Context, loanID string, refund mgpoller.RefundRecord) error {
	if refund.TxHash == "" {
		return pollerAdapterErr("record_refund").With(pkgErrors.AttrLoanID, loanID).
			Code(pkgErrors.CodeIncompleteResponse).Errorf("refund transaction hash is empty")
	}

	now := time.Now()
	req := loan.UpdateLoanRequest{
		RampRefundTxHash: &refund.TxHash,
		RampRefundAmount: &refund.NetStroops,
		RampRefundedAt:   &now,
	}
	// Written unconditionally, including zero: the column doubles as the ops
	// flag, so "settled with nothing outstanding" has to be distinguishable
	// from "never settled".
	req.RampRefundShortfall = &refund.ShortfallStroops

	if _, err := a.loanSvc.Update(ctx, loanID, req); err != nil {
		return pollerAdapterErr("record_refund").With(pkgErrors.AttrLoanID, loanID).
			Code(pkgErrors.CodeStateWriteFailed).Wrapf(err, "could not record the refund")
	}

	a.logger.Info("MoneyGram refund recorded",
		"loan_id", loanID,
		"refund_tx_hash", refund.TxHash,
		"net_stroops", refund.NetStroops,
		"shortfall_stroops", refund.ShortfallStroops)

	a.recordRefundTransaction(ctx, loanID, refund)
	return nil
}

// recordRefundTransaction writes the refund to the transactions ledger. Best
// effort: the loan row is the system of record for the refund, so a failure
// here is logged and never blocks the vault repay behind it.
func (a *MoneyGramPollerAdapter) recordRefundTransaction(ctx context.Context, loanID string, refund mgpoller.RefundRecord) {
	if a.txnSvc == nil {
		return
	}

	loanRow, err := a.repo.GetByID(ctx, loanID)
	if err != nil {
		a.logger.Warn("could not load loan to record refund transaction",
			"loan_id", loanID, "error", err)
		return
	}

	desc := "MoneyGram refund for cancelled cash pickup"
	provider := "moneygram"
	txHash := refund.TxHash

	txnResp, err := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
		UserID:           &loanRow.UserID,
		AccountID:        &loanRow.AccountID,
		LoanID:           &loanID,
		TxType:           txmodels.TxTypeRefund,
		Amount:           refund.NetStroops,
		Asset:            "USDC",
		StellarTxHash:    &txHash,
		ExternalID:       loanRow.RampRequestID,
		ExternalProvider: &provider,
		Description:      &desc,
		// Amount is what landed; the shortfall is what MG kept back. Recorded
		// here as the leg's own account of the discrepancy — loans carries the
		// queryable copy, indexed for the ops sweep.
		Metadata: txMetadata(map[string]any{
			"shortfall_stroops": refund.ShortfallStroops,
			"principal_stroops": loanRow.PrincipalAmount,
			"refunded_send_tx":  loanRow.RampStellarTxHash,
		}),
	})
	if err != nil {
		a.logger.Warn("failed to record refund transaction",
			"loan_id", loanID, "error", err)
		return
	}

	// The refund is already confirmed on-ledger by the time we get here.
	if txnResp != nil {
		a.settleTransaction(ctx, txnResp.ID, "refund", loanID, txmodels.TxStatusSuccess)
	}
}

// settleTransaction advances a freshly created row to its final status.
//
// The transaction service rejects pending -> success outright: its transition
// table only allows pending -> submitted -> success. A single update to
// success therefore returned ErrInvalidStatusTransition, which these
// best-effort callers logged and swallowed, leaving settled payments recorded
// as pending forever.
func (a *MoneyGramPollerAdapter) settleTransaction(ctx context.Context, txnID, kind, loanID, final string) {
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

// projectLoanRecord maps a Loan row into the mgpoller projection. Pointer
// fields default to their zero values when nil — the poller skips records
// without a MoneyGramTxID, which catches the case where Initiate hasn't
// populated RampRequestID yet.
func projectLoanRecord(l *models.Loan) mgpoller.LoanRecord {
	rec := mgpoller.LoanRecord{
		LoanID:           l.ID,
		UserID:           l.UserID,
		PrincipalStroops: l.PrincipalAmount,
	}
	if l.RampSequenceID != nil {
		rec.SequenceID = *l.RampSequenceID
	}
	if l.RampRequestID != nil {
		rec.MoneyGramTxID = *l.RampRequestID
	}
	if l.RampChildAccountIndex != nil {
		// Stored as int64 to fit the column, fits a uint32 by construction.
		rec.ChildAccountIndex = uint32(*l.RampChildAccountIndex)
	}
	if l.RequestedLocalAmount != nil {
		// Persisted as cents; the poller's drift check works in major units.
		rec.RequestedLocalAmount = float64(*l.RequestedLocalAmount) / 100.0
	}
	// Derived, not stored: see models.Loan.DeriveDisbursementStatus. Computed
	// from the batch snapshot, so it reflects state before this tick's writes —
	// which is what the poller's once-only transitions compare against.
	rec.DisbursementStatus = l.DeriveDisbursementStatus()
	// Local proof we already paid MG's anchor. Authoritative over MG's
	// stellar_transaction_id, which can lag minutes behind the payment.
	rec.HasStellarSend = l.RampStellarTxHash != nil && *l.RampStellarTxHash != ""
	if l.User != nil {
		rec.PhoneNumber = l.User.MobileNumber
	}
	return rec
}

// decimalToCents converts a SEP-24 decimal string (e.g. "1250.00") to an
// int64 in cents. Returns (0, false) on malformed input — callers should
// skip the field rather than write a zero.
func decimalToCents(s string) (int64, bool) {
	var v float64
	if _, err := fmt.Sscanf(s, "%f", &v); err != nil {
		return 0, false
	}
	if v < 0 {
		return 0, false
	}
	return int64(v * 100), true
}

// needsMoreInfoShortCode reports whether the loan still lacks a support-link
// code. Errs toward "already has one": minting a duplicate would orphan a code
// the borrower may already hold.
func (a *MoneyGramPollerAdapter) needsMoreInfoShortCode(ctx context.Context, loanID string) bool {
	loanRow, err := a.repo.GetByID(ctx, loanID)
	if err != nil {
		a.logger.Warn("could not load loan to check more-info short code",
			"loan_id", loanID, "error", err)
		return false
	}
	return loanRow.RampMoreInfoShortCode == nil || *loanRow.RampMoreInfoShortCode == ""
}
