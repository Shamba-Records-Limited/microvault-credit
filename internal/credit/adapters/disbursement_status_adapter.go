package adapters

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/pkg/notifications"
	"github.com/Shamba-Records-Limited/microvault/pkg/webhook"
)

// Compile-time checks.
var (
	_ webhook.DisbursementUpdater   = (*DisbursementStatusAdapter)(nil)
	_ webhook.RefundPendingFetcher = (*DisbursementStatusAdapter)(nil)
)

// DisbursementStatusAdapter implements webhook.DisbursementUpdater and
// webhook.RefundPendingFetcher using the credit loan repository.
type DisbursementStatusAdapter struct {
	repo            repository.LoanRepository
	notificationSvc *notifications.SMSNotificationService
	logger          *slog.Logger
}

// NewDisbursementStatusAdapter creates a new DisbursementStatusAdapter.
func NewDisbursementStatusAdapter(
	repo repository.LoanRepository,
	notificationSvc *notifications.SMSNotificationService,
	logger *slog.Logger,
) *DisbursementStatusAdapter {
	return &DisbursementStatusAdapter{
		repo:            repo,
		notificationSvc: notificationSvc,
		logger:          logger,
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

	if a.notificationSvc == nil {
		a.logger.Warn("notification service not configured, skipping completion SMS",
			"loan_id", loan.ID,
		)
		return nil
	}

	loanNumber := loan.ID
	if loan.LoanNumber != nil {
		loanNumber = *loan.LoanNumber
	}

	amountKES := float64(0)
	if loan.RampFiatAmount != nil {
		amountKES = float64(*loan.RampFiatAmount) / 100
	}

	phone := ""
	if loan.User != nil {
		phone = loan.User.MobileNumber
	}

	if err := a.notificationSvc.SendLoanDisbursementNotification(ctx, phone, loanNumber, amountKES); err != nil {
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

	if a.notificationSvc == nil {
		a.logger.Warn("notification service not configured, skipping failure SMS",
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

	if err := a.notificationSvc.SendLoanDefaultNotification(ctx, phone, loanNumber); err != nil {
		a.logger.Warn("failed to send failure SMS",
			"loan_id", loan.ID,
			"error", err,
		)
		return err
	}

	a.logger.Info("disbursement failure SMS sent", "loan_id", loan.ID)
	return nil
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
		records = append(records, webhook.RefundPendingRecord{
			SequenceID: *l.RampSequenceID,
			PaymentID:  *l.RampRequestID,
		})
	}

	return records, nil
}
