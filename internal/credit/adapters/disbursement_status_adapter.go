package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	txmodels "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/yellowcard"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"
	"github.com/Shamba-Records-Limited/microvault/pkg/webhook"
)

// Compile-time checks.
var (
	_ webhook.DisbursementUpdater  = (*DisbursementStatusAdapter)(nil)
	_ webhook.RefundPendingFetcher = (*DisbursementStatusAdapter)(nil)
	_ webhook.TransactionRecorder  = (*DisbursementStatusAdapter)(nil)
)

// DisbursementStatusAdapter implements webhook.DisbursementUpdater,
// webhook.RefundPendingFetcher, and webhook.TransactionRecorder using
// the credit loan repository and transaction service.
type DisbursementStatusAdapter struct {
	publicBaseURL string // origin for SMS short-links; optional
	repo          repository.LoanRepository
	loanNotifier  contracts.LoanNotifier
	txnSvc        transaction.Service
	stellarSvc    stellar.Service
	logger        *slog.Logger
}

// notifyAsync sends a borrower notification off the caller's thread.
//
// These methods are driven by the MoneyGram poller, whose poll() loop walks
// the batch serially — so a synchronous send made every other loan in the
// batch wait behind one SMS, including treasury transfers. With provider-level
// retries a stalled gateway could hold a tick for minutes.
//
// Every caller already treats delivery as best effort and only logs the error,
// so returning before the send completes loses nothing. See notifyLeakGuard:
// the timeout is a backstop, never a delivery deadline.
func (a *DisbursementStatusAdapter) notifyAsync(label, loanID string, send func(ctx context.Context) error) {
	if a.loanNotifier == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), notifyLeakGuard)
		defer cancel()
		if err := send(ctx); err != nil {
			a.logger.Warn("borrower notification failed",
				"notification", label, "loan_id", loanID, "error", err)
			return
		}
		a.logger.Info("borrower notification sent", "notification", label, "loan_id", loanID)
	}()
}

// SetPublicBaseURL sets the externally-reachable origin used to build the
// /r/{code} support link carried in the cash-pickup ready SMS. When unset the
// SMS omits the link rather than sending a bare path.
func (a *DisbursementStatusAdapter) SetPublicBaseURL(url string) {
	a.publicBaseURL = url
}

// moreInfoLink returns the borrower-facing support link, or "" when the loan
// has no code yet or no origin is configured.
func (a *DisbursementStatusAdapter) moreInfoLink(loan *models.Loan) string {
	if a.publicBaseURL == "" || loan.RampMoreInfoShortCode == nil || *loan.RampMoreInfoShortCode == "" {
		return ""
	}
	return a.publicBaseURL + "/r/" + *loan.RampMoreInfoShortCode
}

// NewDisbursementStatusAdapter creates a new DisbursementStatusAdapter.
func NewDisbursementStatusAdapter(
	repo repository.LoanRepository,
	loanNotifier contracts.LoanNotifier,
	txnSvc transaction.Service,
	stellarSvc stellar.Service,
	logger *slog.Logger,
) *DisbursementStatusAdapter {
	return &DisbursementStatusAdapter{
		repo:         repo,
		loanNotifier: loanNotifier,
		txnSvc:       txnSvc,
		stellarSvc:   stellarSvc,
		logger:       logger,
	}
}

// UpdateDisbursementStatus updates the disbursement status for a loan identified by sequenceID.
func (a *DisbursementStatusAdapter) UpdateDisbursementStatus(sequenceID string, status string) error {
	ctx := context.Background()

	loan, err := a.repo.GetBySequenceID(ctx, sequenceID)
	if err != nil {
		a.logger.Error("failed to find loan by sequence ID",
			"sequence_id", sequenceID,
			"error", err,
		)
		return fmt.Errorf("find loan by sequence %s: %w", sequenceID, err)
	}

	status = canonicalDisbursementStatus(status)

	// Disbursement state is derived, so a status update writes the facts it
	// implies rather than a label. Terminal outcomes move the loan's own
	// status; the two states that are not derivable get their markers stamped.
	if next := loanStatusForDisbursement(status); next != "" {
		loan.Status = next
	}
	now := time.Now()
	switch status {
	case models.DisbursementStatusRefundPending:
		if loan.RampRefundDeclaredAt == nil {
			loan.RampRefundDeclaredAt = &now
		}
	case models.DisbursementStatusProcessing:
		if loan.RampPickupReadyAt == nil {
			loan.RampPickupReadyAt = &now
		}
	}

	if err := a.repo.Update(ctx, loan); err != nil {
		a.logger.Error("failed to update disbursement status",
			"loan_id", loan.ID,
			"sequence_id", sequenceID,
			"status", status,
			"error", err,
		)
		return fmt.Errorf("update disbursement status for loan %s: %w", loan.ID, err)
	}

	a.logger.Info("disbursement status updated",
		"loan_id", loan.ID,
		"sequence_id", sequenceID,
		"status", status,
	)

	// Sync transaction status for off-ramp transactions.
	if a.txnSvc != nil {
		txStatus := mapDisbursementToTxStatus(status)
		if txStatus != "" {
			if err := a.UpdateOffRampTransaction(ctx, loan.ID, txStatus, status); err != nil {
				a.logger.Warn("failed to sync transaction status", "loan_id", loan.ID, "error", err)
			}
		}
	}

	// Trigger vault repay when USDC is confirmed to still be in treasury.
	switch status {
	case models.DisbursementStatusCompleted:
		// Fiat complete: YC fronted fiat, USDC still in treasury to repay.
		// Direct complete: USDC sent to YC wallet to do NOT repay.
		if loan.SettlementMethod != nil && *loan.SettlementMethod == "fiat" {
			_ = a.repayVaultIfNeeded(ctx, loan, "fiat_complete", nil)
		}
	case models.DisbursementStatusFailed:
		// Either path: the borrowed USDC is back in (or was never out of)
		// treasury, so repay the vault. repayVaultIfNeeded is idempotent via
		// VaultRepayTxHash, so a later RefundPoller cycle won't double-repay.
		// If USDC is still in flight (e.g. direct failed pre-refund), the
		// on-chain RepayToVault call will fail; the RefundPoller will retry
		// once YC returns the USDC.
		trigger := "fiat_failed"
		if loan.SettlementMethod != nil && *loan.SettlementMethod == "direct" {
			trigger = "direct_failed"
		}
		_ = a.repayVaultIfNeeded(ctx, loan, trigger, nil)
	}

	return nil
}

// repayVaultIfNeeded checks whether USDC is still in the treasury for this loan
// and, if so, calls RepayToVault to return it to the pool. Idempotent: skips if
// VaultRepayTxHash is already set.
//
// amountOverride repays a specific stroop amount instead of the loan principal.
// Refunds need this: an anchor may return less than we sent, and repaying the
// full principal would draw the difference from unrelated treasury funds.
func (a *DisbursementStatusAdapter) repayVaultIfNeeded(ctx context.Context, loan *models.Loan, trigger string, amountOverride *int64) error {
	// Idempotency: already repaid.
	if loan.VaultRepayTxHash != nil && *loan.VaultRepayTxHash != "" {
		a.logger.Info("vault repay already completed, skipping",
			"loan_id", loan.ID,
			"repay_tx_hash", *loan.VaultRepayTxHash,
		)
		return nil
	}

	// No vault borrow happened — nothing to repay.
	if loan.VaultTxHash == nil || *loan.VaultTxHash == "" {
		return nil
	}

	if a.stellarSvc == nil {
		a.logger.Error("stellar service not configured, cannot repay vault", "loan_id", loan.ID)
		return fmt.Errorf("stellar service not configured")
	}

	amount := loan.PrincipalAmount
	if amountOverride != nil {
		amount = *amountOverride
	}
	if amount <= 0 {
		return fmt.Errorf("refusing to repay a non-positive amount (%d stroops)", amount)
	}
	a.logger.Info("initiating vault repay",
		"loan_id", loan.ID,
		"amount_stroops", amount,
		"trigger", trigger,
		"settlement_method", loan.SettlementMethod,
	)

	repayResp, err := a.stellarSvc.RepayToVault(ctx, stellar.RepayRequest{Amount: amount})
	if err != nil {
		a.logger.Error("CRITICAL: vault repay failed — USDC stuck in treasury",
			"loan_id", loan.ID,
			"amount_stroops", amount,
			"trigger", trigger,
			"error", err,
		)
		// Stamp vault_repay_status=failed so ops can query and retry.
		// repay_tx_hash stays NULL — the (failed, null hash) combination
		// is the signal for "needs manual intervention or sweep".
		failedStatus := models.VaultRepayStatusFailed
		loan.VaultRepayStatus = &failedStatus
		if upErr := a.repo.Update(ctx, loan); upErr != nil {
			a.logger.Error("failed to save vault_repay_status=failed",
				"loan_id", loan.ID,
				"error", upErr,
			)
		}
		return fmt.Errorf("repay to vault: %w", err)
	}

	// Persist repay tx hash + success status on the loan.
	loan.VaultRepayTxHash = &repayResp.TxHash
	successStatus := models.VaultRepayStatusSuccess
	loan.VaultRepayStatus = &successStatus
	if err := a.repo.Update(ctx, loan); err != nil {
		a.logger.Error("failed to save vault repay tx hash",
			"loan_id", loan.ID,
			"repay_tx_hash", repayResp.TxHash,
			"error", err,
		)
	}

	// Record vault_repay transaction for audit trail.
	if a.txnSvc != nil {
		desc := "Vault repay"
		txnResp, txnErr := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
			UserID:           &loan.UserID,
			LoanID:           &loan.ID,
			TxType:           txmodels.TxTypeVaultRepay,
			Amount:           repayResp.AmountRepaid,
			Asset:            "USDC",
			StellarTxHash:    &repayResp.TxHash,
			StellarLedger:    &repayResp.Ledger,
			ContractID:       &repayResp.ContractID,
			ContractFunction: &repayResp.ContractFunction,
			Description:      &desc,
			Metadata:         txMetadata(map[string]any{"trigger": trigger}),
		})
		if txnErr != nil {
			a.logger.Warn("failed to record vault repay transaction",
				"loan_id", loan.ID,
				"error", txnErr,
			)
		} else if txnResp != nil {
			submittedStatus := txmodels.TxStatusSubmitted
			_, _ = a.txnSvc.Update(ctx, txnResp.ID, transaction.UpdateTransactionRequest{
				Status: &submittedStatus,
			})
			successStatus := txmodels.TxStatusSuccess
			_, _ = a.txnSvc.Update(ctx, txnResp.ID, transaction.UpdateTransactionRequest{
				Status:        &successStatus,
				StellarLedger: &repayResp.Ledger,
			})
		}
	}

	a.logger.Info("vault repay completed",
		"loan_id", loan.ID,
		"repay_tx_hash", repayResp.TxHash,
		"amount_repaid", repayResp.AmountRepaid,
		"trigger", trigger,
	)
	return nil
}

// RecordDisbursementCompletion persists the final financials of a completed
// payment: delivered_amount_local (convertedAmount, the gross fiat the borrower
// received) and the service/partner fees, all in cents. Idempotent: skips when
// DeliveredAmountLocal is already set, so a replayed DisbursementComplete webhook
// won't overwrite.
func (a *DisbursementStatusAdapter) RecordDisbursementCompletion(sequenceID string, fin webhook.CompletionFinancials) error {
	ctx := context.Background()
	loan, err := a.repo.GetBySequenceID(ctx, sequenceID)
	if err != nil {
		return fmt.Errorf("completion financials: find loan by sequence %s: %w", sequenceID, err)
	}
	if loan.DeliveredAmountLocal != nil {
		return nil
	}

	delivered := majorToCents(fin.ConvertedAmountLocal)
	serviceFeeUSD := majorToCents(fin.ServiceFeeAmountUSD)
	serviceFeeLocal := majorToCents(fin.ServiceFeeAmountLocal)
	partnerFeeUSD := majorToCents(fin.PartnerFeeAmountUSD)
	partnerFeeLocal := majorToCents(fin.PartnerFeeAmountLocal)

	loan.DeliveredAmountLocal = &delivered
	loan.ServiceFeeUSD = &serviceFeeUSD
	loan.ServiceFeeLocal = &serviceFeeLocal
	loan.PartnerFeeUSD = &partnerFeeUSD
	loan.PartnerFeeLocal = &partnerFeeLocal

	if err := a.repo.Update(ctx, loan); err != nil {
		return fmt.Errorf("completion financials: persist loan %s: %w", loan.ID, err)
	}
	a.logger.Info("disbursement completion financials recorded",
		"loan_id", loan.ID,
		"delivered_local_cents", delivered,
		"service_fee_local_cents", serviceFeeLocal,
		"partner_fee_local_cents", partnerFeeLocal,
	)
	return nil
}

// majorToCents converts a major-unit amount (e.g. 29.99 USD) to integer cents.
func majorToCents(v float64) int64 {
	return int64(v*100 + 0.5)
}

// IsDirectSettlement reports whether the loan identified by sequenceID is
// in direct-settlement mode. Used by the YC webhook handler to decide
// whether a FAILED event should mark the loan refund_pending (direct, USDC
// awaits crypto refund) or terminal_failed (fiat). A NULL settlement_method
// is treated as not-direct — safer to mark terminal_failed than wait for a
// refund that will never come.
func (a *DisbursementStatusAdapter) IsDirectSettlement(sequenceID string) (bool, error) {
	ctx := context.Background()
	loan, err := a.repo.GetBySequenceID(ctx, sequenceID)
	if err != nil {
		return false, fmt.Errorf("find loan by sequence %s: %w", sequenceID, err)
	}
	if loan.SettlementMethod == nil {
		return false, nil
	}
	return *loan.SettlementMethod == "direct", nil
}

// SetSettlementMethod updates the loan's settlement_method field. Used by
// the RefundPoller when a direct-mode disbursement is failed over to fiat:
// without this flip, the eventual DisbursementComplete event would see
// settlement_method="direct" and skip the vault repay branch.
func (a *DisbursementStatusAdapter) SetSettlementMethod(sequenceID string, method string) error {
	ctx := context.Background()
	loan, err := a.repo.GetBySequenceID(ctx, sequenceID)
	if err != nil {
		return fmt.Errorf("find loan by sequence %s: %w", sequenceID, err)
	}
	loan.SettlementMethod = &method
	if err := a.repo.Update(ctx, loan); err != nil {
		return fmt.Errorf("update settlement_method for loan %s: %w", loan.ID, err)
	}
	a.logger.Info("settlement_method updated",
		"loan_id", loan.ID,
		"sequence_id", sequenceID,
		"method", method,
	)
	return nil
}

// RepayVault returns borrowed USDC from treasury to the vault pool for the
// loan identified by sequenceID. No-op if already repaid.
func (a *DisbursementStatusAdapter) RepayVault(sequenceID string) error {
	ctx := context.Background()
	loan, err := a.repo.GetBySequenceID(ctx, sequenceID)
	if err != nil {
		return fmt.Errorf("find loan by sequence %s: %w", sequenceID, err)
	}
	_ = a.repayVaultIfNeeded(ctx, loan, "explicit_repay", nil)
	return nil
}

// RepayVaultAmount returns an explicit stroop amount to the vault rather than
// the loan principal, and unlike RepayVault it surfaces the failure.
//
// Used for anchor refunds, where the amount that came back is authoritative:
// repaying the principal when the anchor withheld a fee would draw the shortfall
// from unrelated treasury funds. The caller retries on error.
func (a *DisbursementStatusAdapter) RepayVaultAmount(sequenceID string, amountStroops int64) error {
	ctx := context.Background()
	loan, err := a.repo.GetBySequenceID(ctx, sequenceID)
	if err != nil {
		return fmt.Errorf("find loan by sequence %s: %w", sequenceID, err)
	}
	return a.repayVaultIfNeeded(ctx, loan, "anchor_refund", &amountStroops)
}

// NotifyDisbursementComplete sends an SMS notification that the disbursement completed.
func (a *DisbursementStatusAdapter) NotifyDisbursementComplete(sequenceID string) error {
	ctx := context.Background()

	loan, err := a.repo.GetBySequenceID(ctx, sequenceID)
	if err != nil {
		a.logger.Error("failed to find loan for completion notification",
			"sequence_id", sequenceID,
			"error", err,
		)
		return fmt.Errorf("find loan by sequence %s: %w", sequenceID, err)
	}

	if a.loanNotifier == nil {
		a.logger.Warn("loan notifier not configured, skipping completion SMS",
			"loan_id", loan.ID,
		)
		return nil
	}

	loanRef := loan.ID
	if loan.LoanReference != nil {
		loanRef = *loan.LoanReference
	}

	// Prefer delivered_amount_local (net of provider fees) over ramp_fiat_amount
	// so the SMS reflects what the borrower actually received. Fall back to
	// the gross amount when delivered hasn't been recorded yet (e.g. webhook
	// arrived out of order).
	displayAmount := float64(0)
	displayCurrency := "KES"
	switch {
	case loan.DeliveredAmountLocal != nil:
		displayAmount = float64(*loan.DeliveredAmountLocal) / 100
	case loan.RampFiatAmount != nil:
		displayAmount = float64(*loan.RampFiatAmount) / 100
	}
	if loan.RampFiatCurr != nil {
		displayCurrency = *loan.RampFiatCurr
	}

	phone := ""
	if loan.User != nil {
		phone = loan.User.MobileNumber
	}

	note := contracts.LoanNotification{
		LoanID:          loan.ID,
		LoanReference:   loanRef,
		PhoneNumber:     phone,
		DisplayAmount:   displayAmount,
		DisplayCurrency: displayCurrency,
	}
	a.notifyAsync("disbursement_complete", loan.ID, func(ctx context.Context) error {
		return a.loanNotifier.NotifyLoanDisbursed(ctx, note)
	})
	return nil
}

// NotifyCashPickupReady sends the borrower their MoneyGram pickup reference
// once the anchor confirms the cash is collectable. Called by the MG poller on
// pending_user_transfer_complete.
func (a *DisbursementStatusAdapter) NotifyCashPickupReady(sequenceID string) error {
	ctx := context.Background()

	loan, err := a.repo.GetBySequenceID(ctx, sequenceID)
	if err != nil {
		a.logger.Error("failed to find loan for cash-pickup ready notification",
			"sequence_id", sequenceID,
			"error", err,
		)
		return fmt.Errorf("find loan by sequence %s: %w", sequenceID, err)
	}

	if a.loanNotifier == nil {
		a.logger.Warn("loan notifier not configured, skipping cash-pickup ready SMS",
			"loan_id", loan.ID,
		)
		return nil
	}

	loanRef := loan.ID
	if loan.LoanReference != nil {
		loanRef = *loan.LoanReference
	}

	// The reference the borrower quotes at the agent. Without it the SMS is
	// not actionable, so skip rather than send a half-useful message — the
	// poller keeps ticking and will retry once MG supplies it.
	if loan.RampExternalRef == nil || *loan.RampExternalRef == "" {
		a.logger.Warn("no MoneyGram reference yet, deferring cash-pickup ready SMS",
			"loan_id", loan.ID,
		)
		return fmt.Errorf("loan %s has no ramp_external_ref", loan.ID)
	}

	displayAmount := float64(0)
	displayCurrency := "KES"
	switch {
	case loan.DeliveredAmountLocal != nil:
		displayAmount = float64(*loan.DeliveredAmountLocal) / 100
	case loan.RampFiatAmount != nil:
		displayAmount = float64(*loan.RampFiatAmount) / 100
	}
	if loan.RampFiatCurr != nil {
		displayCurrency = *loan.RampFiatCurr
	}

	phone := ""
	if loan.User != nil {
		phone = loan.User.MobileNumber
	}

	note := contracts.LoanNotification{
		LoanID:            loan.ID,
		LoanReference:     loanRef,
		PhoneNumber:       phone,
		DisplayAmount:     displayAmount,
		DisplayCurrency:   displayCurrency,
		CashPickupRef:     *loan.RampExternalRef,
		CashPickupInfoURL: a.moreInfoLink(loan),
	}
	a.notifyAsync("cash_pickup_ready", loan.ID, func(ctx context.Context) error {
		return a.loanNotifier.NotifyLoanCashPickupReady(ctx, note)
	})
	return nil
}

// NotifyRefundReceived tells the borrower their cash pickup was cancelled and
// the funds returned.
func (a *DisbursementStatusAdapter) NotifyRefundReceived(sequenceID string) error {
	ctx := context.Background()

	loan, err := a.repo.GetBySequenceID(ctx, sequenceID)
	if err != nil {
		return fmt.Errorf("find loan by sequence %s: %w", sequenceID, err)
	}

	if a.loanNotifier == nil {
		a.logger.Warn("loan notifier not configured, skipping refund SMS", "loan_id", loan.ID)
		return nil
	}

	loanRef := loan.ID
	if loan.LoanReference != nil {
		loanRef = *loan.LoanReference
	}
	phone := ""
	if loan.User != nil {
		phone = loan.User.MobileNumber
	}

	note := contracts.LoanNotification{
		LoanID:        loan.ID,
		LoanReference: loanRef,
		PhoneNumber:   phone,
	}
	a.notifyAsync("refund_received", loan.ID, func(ctx context.Context) error {
		return a.loanNotifier.NotifyLoanCashPickupCancelled(ctx, note)
	})
	return nil
}

// NotifyDisbursementFailed sends an SMS notification that the disbursement failed.
func (a *DisbursementStatusAdapter) NotifyDisbursementFailed(sequenceID string) error {
	ctx := context.Background()

	loan, err := a.repo.GetBySequenceID(ctx, sequenceID)
	if err != nil {
		a.logger.Error("failed to find loan for failure notification",
			"sequence_id", sequenceID,
			"error", err,
		)
		return fmt.Errorf("find loan by sequence %s: %w", sequenceID, err)
	}

	if a.loanNotifier == nil {
		a.logger.Warn("loan notifier not configured, skipping failure SMS",
			"loan_id", loan.ID,
		)
		return nil
	}

	loanRef := loan.ID
	if loan.LoanReference != nil {
		loanRef = *loan.LoanReference
	}

	phone := ""
	if loan.User != nil {
		phone = loan.User.MobileNumber
	}

	note := contracts.LoanNotification{
		LoanID:        loan.ID,
		LoanReference: loanRef,
		PhoneNumber:   phone,
	}
	a.notifyAsync("disbursement_failed", loan.ID, func(ctx context.Context) error {
		return a.loanNotifier.NotifyLoanFailed(ctx, note)
	})
	return nil
}

// UpdateOffRampTransaction syncs the loan's off-ramp row with the provider's
// reported status.
//
// Resolved by loan and type rather than by external ID: every leg of an anchor
// transaction now shares the provider's request ID, so that lookup no longer
// identifies one row.
func (a *DisbursementStatusAdapter) UpdateOffRampTransaction(ctx context.Context, loanID string, status string, externalStatus string) error {
	txnResp, err := a.txnSvc.GetByLoanIDAndType(ctx, loanID, txmodels.TxTypeOffRamp)
	if err != nil || txnResp == nil {
		a.logger.Debug("no off-ramp transaction found for loan", "loan_id", loanID)
		return nil
	}

	_, err = a.txnSvc.Update(ctx, txnResp.ID, transaction.UpdateTransactionRequest{
		Status:         &status,
		ExternalStatus: &externalStatus,
	})
	return err
}

// RecordFiatFailover records a fiat failover transaction after a direct settlement refund.
func (a *DisbursementStatusAdapter) RecordFiatFailover(ctx context.Context, rec webhook.RefundPendingRecord, newRequestID string) error {
	provider := "yellowcard"
	desc := "Fiat failover after direct settlement refund"

	asset := rec.RampFiatCurrency
	if asset == "" {
		asset = "KES"
	}

	_, err := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
		UserID:           &rec.UserID,
		LoanID:           &rec.LoanID,
		TxType:           txmodels.TxTypeFiatFailover,
		Amount:           rec.RampFiatAmount,
		Asset:            asset,
		ExternalID:       &newRequestID,
		ExternalProvider: &provider,
		Description:      &desc,
		Metadata:         txMetadata(map[string]any{"original_request_id": rec.PaymentID}),
	})
	return err
}

// mapDisbursementToTxStatus maps YellowCard disbursement statuses to transaction statuses.
func mapDisbursementToTxStatus(disbursementStatus string) string {
	// Canonical values, not YellowCard's wire ones: callers pass the output of
	// canonicalDisbursementStatus, so "complete" never arrives here.
	switch disbursementStatus {
	case models.DisbursementStatusCompleted:
		return txmodels.TxStatusSuccess
	case models.DisbursementStatusFailed:
		return txmodels.TxStatusFailed
	case models.DisbursementStatusProcessing, yellowcard.DisbursementDirectSubmitted:
		return txmodels.TxStatusSubmitted
	case models.DisbursementStatusRefundPending, models.DisbursementStatusRefundReceived:
		return txmodels.TxStatusPending
	default:
		return ""
	}
}

// GetRefundPendingDisbursements returns YellowCard disbursements awaiting
// crypto refund.
//
// Scoped to yellowcard deliberately. RefundPoller resolves each record against
// the YellowCard API using RampRequestID, so a MoneyGram loan sitting in
// refund_pending would be looked up with an MG transaction ID — which at best
// errors every cycle and at worst matches an unrelated YC payment and triggers
// a mobile-money failover for a cash-pickup loan. MoneyGram refunds are the
// MG poller's responsibility.
func (a *DisbursementStatusAdapter) GetRefundPendingDisbursements() ([]webhook.RefundPendingRecord, error) {
	ctx := context.Background()

	loans, err := a.repo.GetRefundDeclared(ctx, "yellowcard", 100)
	if err != nil {
		a.logger.Error("failed to fetch refund pending disbursements", "error", err)
		return nil, fmt.Errorf("fetch refund pending: %w", err)
	}

	records := make([]webhook.RefundPendingRecord, 0, len(loans))
	for _, l := range loans {
		if l.RampSequenceID == nil || l.RampRequestID == nil {
			continue
		}

		var phone, name, country, netCode, netName string
		if l.User != nil {
			phone = l.User.MobileNumber
			if l.User.FullName != nil {
				name = *l.User.FullName
			}
			country = l.User.CountryCode
			netCode = l.User.MomoNetworkCode
			netName = l.User.MomoNetworkName
		}

		amountUSD := float64(l.PrincipalAmount) / 1e7

		var rampFiatAmount int64
		var rampFiatCurrency string
		if l.RampFiatAmount != nil {
			rampFiatAmount = *l.RampFiatAmount
		}
		if l.RampFiatCurr != nil {
			rampFiatCurrency = *l.RampFiatCurr
		}

		records = append(records, webhook.RefundPendingRecord{
			SequenceID:       *l.RampSequenceID,
			PaymentID:        *l.RampRequestID,
			LoanID:           l.ID,
			UserID:           l.UserID,
			RecipientName:    name,
			AmountUSD:        amountUSD,
			AmountStroops:    l.PrincipalAmount,
			RampFiatAmount:   rampFiatAmount,
			RampFiatCurrency: rampFiatCurrency,
			DestinationPhone: phone,
			CountryCode:      country,
			NetworkCode:      netCode,
			NetworkName:      netName,
		})
	}

	return records, nil
}

// canonicalDisbursementStatus maps a provider's wire value onto the loan
// model's vocabulary.
//
// YellowCard reports completion as "complete" while MoneyGram and the model
// use "completed". Both reached this adapter untranslated, so a MoneyGram
// completion never matched the YellowCard constant and never advanced the loan
// — every cash-pickup loan stayed "disbursing" for life, including refunded
// ones, which left them inside GetActiveLoans.
func canonicalDisbursementStatus(status string) string {
	if status == yellowcard.DisbursementComplete {
		return models.DisbursementStatusCompleted
	}
	return status
}

// loanStatusForDisbursement returns the loan status a terminal payout outcome
// implies, or "" when the outcome is not terminal and the loan status stands.
//
// A settled refund is LoanStatusCancelled: the borrower received nothing and
// owes nothing. How it ended is already recorded in disbursement_status, so a
// separate refunded loan status would differ only in provenance.
func loanStatusForDisbursement(status string) string {
	switch status {
	case models.DisbursementStatusCompleted:
		return models.LoanStatusDisbursed
	case models.DisbursementStatusFailed:
		return models.LoanStatusOffRampFailed
	case models.DisbursementStatusRefundReceived:
		return models.LoanStatusCancelled
	default:
		return ""
	}
}
