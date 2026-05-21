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
			a.repayVaultIfNeeded(ctx, loan, "fiat_complete")
		}
		// Stamp delivered_amount_kes = ramp_fiat_amount - ramp_fee_local
		// (both in cents). This is the actual fiat the borrower received;
		// downstream SMS + UI should prefer this over ramp_fiat_amount.
		a.recordDeliveredAmount(ctx, loan)
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
		a.repayVaultIfNeeded(ctx, loan, trigger)
	}

	return nil
}

// repayVaultIfNeeded checks whether USDC is still in the treasury for this loan
// and, if so, calls RepayToVault to return it to the pool. Idempotent: skips if
// VaultRepayTxHash is already set.
func (a *DisbursementStatusAdapter) repayVaultIfNeeded(ctx context.Context, loan *models.Loan, trigger string) {
	// Idempotency: already repaid.
	if loan.VaultRepayTxHash != nil && *loan.VaultRepayTxHash != "" {
		a.logger.Info("vault repay already completed, skipping",
			"loan_id", loan.ID,
			"repay_tx_hash", *loan.VaultRepayTxHash,
		)
		return
	}

	// No vault borrow happened — nothing to repay.
	if loan.VaultTxHash == nil || *loan.VaultTxHash == "" {
		return
	}

	if a.stellarSvc == nil {
		a.logger.Error("stellar service not configured, cannot repay vault", "loan_id", loan.ID)
		return
	}

	amount := loan.PrincipalAmount
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
		return
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
			UserID:        &loan.UserID,
			LoanID:        &loan.ID,
			TxType:        txmodels.TxTypeVaultRepay,
			TxCategory:    txmodels.TxCategoryOnChain,
			Amount:        repayResp.AmountRepaid,
			Asset:         "USDC",
			StellarTxHash: &repayResp.TxHash,
			Description:   &desc,
		})
		if txnErr != nil {
			a.logger.Warn("failed to record vault repay transaction",
				"loan_id", loan.ID,
				"error", txnErr,
			)
		} else if txnResp != nil {
			s := txmodels.TxStatusSuccess
			_, _ = a.txnSvc.Update(ctx, txnResp.ID, transaction.UpdateTransactionRequest{Status: &s})
		}
	}

	a.logger.Info("vault repay completed",
		"loan_id", loan.ID,
		"repay_tx_hash", repayResp.TxHash,
		"amount_repaid", repayResp.AmountRepaid,
		"trigger", trigger,
	)
}

// recordDeliveredAmount computes and persists the net fiat the borrower
// actually received: ramp_fiat_amount - ramp_fee_local. Both are in cents.
// Idempotent: skips when DeliveredAmtKES is already set, so a replayed
// DisbursementComplete webhook won't overwrite. NULL inputs are treated as
// zero — a missing ramp_fee_local still yields a usable delivered amount.
func (a *DisbursementStatusAdapter) recordDeliveredAmount(ctx context.Context, loan *models.Loan) {
	if loan.DeliveredAmtKES != nil {
		return
	}
	if loan.RampFiatAmount == nil {
		return
	}
	gross := *loan.RampFiatAmount
	fee := int64(0)
	if loan.RampFeeLocal != nil {
		fee = *loan.RampFeeLocal
	}
	delivered := gross - fee
	if delivered < 0 {
		a.logger.Warn("delivered_amount_kes would be negative — skipping",
			"loan_id", loan.ID,
			"ramp_fiat_amount", gross,
			"ramp_fee_local", fee,
		)
		return
	}
	loan.DeliveredAmtKES = &delivered
	if err := a.repo.Update(ctx, loan); err != nil {
		a.logger.Warn("failed to persist delivered_amount_kes",
			"loan_id", loan.ID,
			"error", err,
		)
		return
	}
	a.logger.Info("delivered_amount_kes recorded",
		"loan_id", loan.ID,
		"delivered_kes_cents", delivered,
		"gross_kes_cents", gross,
		"fee_kes_cents", fee,
	)
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
	a.repayVaultIfNeeded(ctx, loan, "explicit_repay")
	return nil
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

// GetRefundPendingDisbursements returns disbursements awaiting crypto refund.
func (a *DisbursementStatusAdapter) GetRefundPendingDisbursements() ([]webhook.RefundPendingRecord, error) {
	ctx := context.Background()

	loans, err := a.repo.GetByDisbursementStatus(ctx, "refund_pending", 100)
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
