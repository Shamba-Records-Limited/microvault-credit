package adapters

import (
	"context"
	"fmt"
	"log/slog"

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
	repo         repository.LoanRepository
	loanNotifier contracts.LoanNotifier
	txnSvc       transaction.Service
	stellarSvc   stellar.Service
	logger       *slog.Logger
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

	loan.DisbursementStatus = &status

	// Sync main loan status with final payout results
	if status == yellowcard.DisbursementComplete {
		loan.Status = models.LoanStatusDisbursed
	} else if status == yellowcard.DisbursementFailed {
		loan.Status = models.LoanStatusOffRampFailed
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
	if loan.RampRequestID != nil && a.txnSvc != nil {
		txStatus := mapDisbursementToTxStatus(status)
		if txStatus != "" {
			if err := a.UpdateTransactionByExternalID(ctx, *loan.RampRequestID, txStatus, status); err != nil {
				a.logger.Warn("failed to sync transaction status", "loan_id", loan.ID, "error", err)
			}
		}
	}

	// Trigger vault repay when USDC is confirmed to still be in treasury.
	switch status {
	case yellowcard.DisbursementComplete:
		// Fiat complete: YC fronted fiat, USDC still in treasury to repay.
		// Direct complete: USDC sent to YC wallet to do NOT repay.
		if loan.SettlementMethod != nil && *loan.SettlementMethod == "fiat" {
			_ = a.repayVaultIfNeeded(ctx, loan, "fiat_complete", nil)
		}
	case yellowcard.DisbursementFailed:
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
		desc := fmt.Sprintf("Vault repay (trigger: %s)", trigger)
		txnResp, txnErr := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
			UserID:           &loan.UserID,
			LoanID:           &loan.ID,
			TxType:           txmodels.TxTypeVaultRepay,
			TxCategory:       txmodels.TxCategoryOnChain,
			Amount:           repayResp.AmountRepaid,
			Asset:            "USDC",
			StellarTxHash:    &repayResp.TxHash,
			StellarLedger:    &repayResp.Ledger,
			ContractID:       &repayResp.ContractID,
			ContractFunction: &repayResp.ContractFunction,
			Description:      &desc,
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
				StellarStatus: &repayResp.Status,
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
// payment: delivered_amount_kes (convertedAmount, the gross fiat the borrower
// received) and the service/partner fees, all in cents. Idempotent: skips when
// DeliveredAmtKES is already set, so a replayed DisbursementComplete webhook
// won't overwrite.
func (a *DisbursementStatusAdapter) RecordDisbursementCompletion(sequenceID string, fin webhook.CompletionFinancials) error {
	ctx := context.Background()
	loan, err := a.repo.GetBySequenceID(ctx, sequenceID)
	if err != nil {
		return fmt.Errorf("completion financials: find loan by sequence %s: %w", sequenceID, err)
	}
	if loan.DeliveredAmtKES != nil {
		return nil
	}

	delivered := majorToCents(fin.ConvertedAmountLocal)
	serviceFeeUSD := majorToCents(fin.ServiceFeeAmountUSD)
	serviceFeeLocal := majorToCents(fin.ServiceFeeAmountLocal)
	partnerFeeUSD := majorToCents(fin.PartnerFeeAmountUSD)
	partnerFeeLocal := majorToCents(fin.PartnerFeeAmountLocal)

	loan.DeliveredAmtKES = &delivered
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

	// Prefer delivered_amount_kes (net of provider fees) over ramp_fiat_amount
	// so the SMS reflects what the borrower actually received. Fall back to
	// the gross amount when delivered hasn't been recorded yet (e.g. webhook
	// arrived out of order).
	displayAmount := float64(0)
	displayCurrency := "KES"
	switch {
	case loan.DeliveredAmtKES != nil:
		displayAmount = float64(*loan.DeliveredAmtKES) / 100
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

	if err := a.loanNotifier.NotifyLoanDisbursed(ctx, contracts.LoanNotification{
		LoanID:          loan.ID,
		LoanReference:   loanRef,
		PhoneNumber:     phone,
		DisplayAmount:   displayAmount,
		DisplayCurrency: displayCurrency,
	}); err != nil {
		a.logger.Warn("failed to send completion SMS",
			"loan_id", loan.ID,
			"error", err,
		)
		return err
	}

	a.logger.Info("disbursement completion SMS sent", "loan_id", loan.ID)
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
	case loan.DeliveredAmtKES != nil:
		displayAmount = float64(*loan.DeliveredAmtKES) / 100
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

	if err := a.loanNotifier.NotifyLoanCashPickupReady(ctx, contracts.LoanNotification{
		LoanID:          loan.ID,
		LoanReference:   loanRef,
		PhoneNumber:     phone,
		DisplayAmount:   displayAmount,
		DisplayCurrency: displayCurrency,
		CashPickupRef:   *loan.RampExternalRef,
	}); err != nil {
		a.logger.Warn("failed to send cash-pickup ready SMS",
			"loan_id", loan.ID,
			"error", err,
		)
		return err
	}

	a.logger.Info("cash-pickup ready SMS sent",
		"loan_id", loan.ID, "reference", *loan.RampExternalRef)
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

	if err := a.loanNotifier.NotifyLoanCashPickupCancelled(ctx, contracts.LoanNotification{
		LoanID:        loan.ID,
		LoanReference: loanRef,
		PhoneNumber:   phone,
	}); err != nil {
		a.logger.Warn("failed to send refund SMS", "loan_id", loan.ID, "error", err)
		return err
	}

	a.logger.Info("refund SMS sent", "loan_id", loan.ID)
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

	if err := a.loanNotifier.NotifyLoanFailed(ctx, contracts.LoanNotification{
		LoanID:        loan.ID,
		LoanReference: loanRef,
		PhoneNumber:   phone,
	}); err != nil {
		a.logger.Warn("failed to send failure SMS",
			"loan_id", loan.ID,
			"error", err,
		)
		return err
	}

	a.logger.Info("disbursement failure SMS sent", "loan_id", loan.ID)
	return nil
}

// UpdateTransactionByExternalID updates an existing transaction found by its external ID.
func (a *DisbursementStatusAdapter) UpdateTransactionByExternalID(ctx context.Context, externalID string, status string, externalStatus string) error {
	txnResp, err := a.txnSvc.GetByExternalID(ctx, externalID)
	if err != nil || txnResp == nil {
		a.logger.Debug("no transaction found for external ID", "external_id", externalID)
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
	desc := fmt.Sprintf("Fiat failover after direct settlement refund (original: %s)", rec.PaymentID)

	asset := rec.RampFiatCurrency
	if asset == "" {
		asset = "KES"
	}

	_, err := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
		UserID:           &rec.UserID,
		LoanID:           &rec.LoanID,
		TxType:           txmodels.TxTypeFiatFailover,
		TxCategory:       txmodels.TxCategoryOffChain,
		Amount:           rec.RampFiatAmount,
		Asset:            asset,
		ExternalID:       &newRequestID,
		ExternalProvider: &provider,
		Description:      &desc,
	})
	return err
}

// mapDisbursementToTxStatus maps YellowCard disbursement statuses to transaction statuses.
func mapDisbursementToTxStatus(disbursementStatus string) string {
	switch disbursementStatus {
	case yellowcard.DisbursementComplete:
		return txmodels.TxStatusSuccess
	case yellowcard.DisbursementFailed:
		return txmodels.TxStatusFailed
	case yellowcard.DisbursementProcessing, yellowcard.DisbursementDirectSubmitted:
		return txmodels.TxStatusSubmitted
	case yellowcard.DisbursementRefundPending, yellowcard.DisbursementRefundReceived:
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

	loans, err := a.repo.GetByDisbursementStatus(ctx, "refund_pending", "yellowcard", 100)
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
