package adapters

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	"github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/yellowcard"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"
	"github.com/Shamba-Records-Limited/microvault/pkg/webhook"
)

// Compile-time checks.
var (
	_ webhook.DisbursementUpdater    = (*DisbursementStatusAdapter)(nil)
	_ webhook.RefundPendingFetcher   = (*DisbursementStatusAdapter)(nil)
	_ webhook.TransactionRecorder    = (*DisbursementStatusAdapter)(nil)
)

// DisbursementStatusAdapter implements webhook.DisbursementUpdater,
// webhook.RefundPendingFetcher, and webhook.TransactionRecorder using
// the credit loan repository and transaction service.
type DisbursementStatusAdapter struct {
	repo         repository.LoanRepository
	loanNotifier contracts.LoanNotifier
	txnSvc       transaction.Service
	logger       *slog.Logger
}

// NewDisbursementStatusAdapter creates a new DisbursementStatusAdapter.
func NewDisbursementStatusAdapter(
	repo repository.LoanRepository,
	loanNotifier contracts.LoanNotifier,
	txnSvc transaction.Service,
	logger *slog.Logger,
) *DisbursementStatusAdapter {
	return &DisbursementStatusAdapter{
		repo:         repo,
		loanNotifier: loanNotifier,
		txnSvc:       txnSvc,
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

	loanNumber := loan.ID
	if loan.LoanNumber != nil {
		loanNumber = *loan.LoanNumber
	}

	displayAmount := float64(0)
	displayCurrency := "KES"
	if loan.RampFiatAmount != nil {
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
		LoanNumber:      loanNumber,
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

	loanNumber := loan.ID
	if loan.LoanNumber != nil {
		loanNumber = *loan.LoanNumber
	}

	phone := ""
	if loan.User != nil {
		phone = loan.User.MobileNumber
	}

	if err := a.loanNotifier.NotifyLoanFailed(ctx, contracts.LoanNotification{
		LoanID:      loan.ID,
		LoanNumber:  loanNumber,
		PhoneNumber: phone,
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
		TxType:           models.TxTypeFiatFailover,
		TxCategory:       models.TxCategoryOffChain,
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
		return models.TxStatusSuccess
	case yellowcard.DisbursementFailed:
		return models.TxStatusFailed
	case yellowcard.DisbursementProcessing, yellowcard.DisbursementDirectSubmitted:
		return models.TxStatusSubmitted
	case yellowcard.DisbursementRefundPending, yellowcard.DisbursementRefundReceived:
		return models.TxStatusPending
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
