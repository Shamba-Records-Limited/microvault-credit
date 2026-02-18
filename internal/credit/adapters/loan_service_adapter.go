// Package adapters bridges microvault-credit services to microvault's USSD interfaces.
package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/pkg/notifications"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	ussdadapters "github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd/adapters"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
)

// Compile-time check.
var _ ussd.LoanService = (*LoanServiceAdapter)(nil)

// LoanServiceAdapter implements ussd.LoanService by orchestrating credit's loan
// service, Stellar vault, YellowCard off-ramp, and SMS notifications.
type LoanServiceAdapter struct {
	loanSvc         loan.Service
	stellarSvc      stellar.Service
	offRampSvc      ussdadapters.OffRampService
	notificationSvc *notifications.SMSNotificationService
	logger          *slog.Logger
	defaultLimit    int64 // auto-approve limit in stroops
}

// NewLoanServiceAdapter creates a new LoanServiceAdapter.
func NewLoanServiceAdapter(
	loanSvc loan.Service,
	stellarSvc stellar.Service,
	offRampSvc ussdadapters.OffRampService,
	notificationSvc *notifications.SMSNotificationService,
	logger *slog.Logger,
	defaultLimit int64,
) *LoanServiceAdapter {
	return &LoanServiceAdapter{
		loanSvc:         loanSvc,
		stellarSvc:      stellarSvc,
		offRampSvc:      offRampSvc,
		notificationSvc: notificationSvc,
		logger:          logger,
		defaultLimit:    defaultLimit,
	}
}

// RequestLoan implements ussd.LoanService. It orchestrates the full loan
// disbursement cycle: eligibility → create → approve → vault borrow → disburse
// → off-ramp → notify.
func (a *LoanServiceAdapter) RequestLoan(ctx context.Context, req *ussd.LoanRequest) (interface{}, error) {
	start := time.Now()

	// Step 1: Auto-approve eligibility check.
	if req.PrincipalAmount > a.defaultLimit {
		a.logger.Warn("loan rejected: exceeds limit",
			"user_id", req.UserID,
			"amount", req.PrincipalAmount,
			"limit", a.defaultLimit,
		)
		return &ussd.LoanApproval{
			Approved: false,
			Reason:   "Loan amount exceeds the current limit",
		}, nil
	}
	a.logger.Info("loan eligibility check",
		"user_id", req.UserID,
		"amount", req.PrincipalAmount,
		"limit", a.defaultLimit,
		"approved", true,
	)

	// Step 2: Create loan record.
	createResp, err := a.loanSvc.Create(ctx, loan.CreateLoanRequest{
		UserID:            req.UserID,
		AccountID:         req.AccountID,
		PrincipalAmount:   req.PrincipalAmount,
		PrincipalAsset:    req.PrincipalAsset,
		InterestRateBps:   500, // 5% default — will be driven by credit scoring later
		DurationDays:      req.DurationDays,
		RepaymentSchedule: req.RepaymentSched,
	})
	if err != nil {
		a.logger.Error("failed to create loan record", "user_id", req.UserID, "error", err)
		return nil, fmt.Errorf("create loan: %w", err)
	}
	loanID := createResp.ID
	a.logger.Info("loan record created",
		"loan_id", loanID,
		"user_id", req.UserID,
		"amount", req.PrincipalAmount,
		"asset", req.PrincipalAsset,
	)

	// Step 3: Auto-approve.
	_, err = a.loanSvc.Approve(ctx, loanID, loan.ApproveLoanRequest{ApprovedBy: "system"})
	if err != nil {
		a.logger.Error("failed to approve loan", "loan_id", loanID, "error", err)
		return nil, fmt.Errorf("approve loan: %w", err)
	}
	a.logger.Info("loan auto-approved", "loan_id", loanID)

	// Step 4: Borrow from Stellar vault.
	borrowResp, err := a.stellarSvc.BorrowFromVault(ctx, stellar.BorrowRequest{
		RecipientAddress: req.AccountID,
		Amount:           req.PrincipalAmount,
	})
	if err != nil {
		a.logger.Error("vault borrow failed", "loan_id", loanID, "error", err)
		// Cancel the loan on vault failure.
		cancelStatus := "cancelled"
		_, _ = a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
			VaultTxStatus: &cancelStatus,
		})
		return nil, fmt.Errorf("vault borrow: %w", err)
	}
	a.logger.Info("vault borrow succeeded",
		"loan_id", loanID,
		"tx_hash", borrowResp.TxHash,
		"amount_borrowed", borrowResp.AmountBorrowed,
	)

	// Step 5: Mark loan as disbursed.
	settlementMethod := "direct"
	disbursementStatus := "crypto_sent"
	_, err = a.loanSvc.Disburse(ctx, loanID, loan.DisburseLoanRequest{
		VaultTxHash:        &borrowResp.TxHash,
		SettlementMethod:   &settlementMethod,
		DisbursementStatus: &disbursementStatus,
	})
	if err != nil {
		// Non-fatal: vault borrow already succeeded.
		a.logger.Error("failed to update loan after disbursement", "loan_id", loanID, "error", err)
	} else {
		a.logger.Info("loan marked disbursed", "loan_id", loanID, "vault_tx_hash", borrowResp.TxHash)
	}

	// Step 6: Initiate off-ramp.
	amountUSD := float64(req.PrincipalAmount) / 1e7
	offRampResult, err := a.offRampSvc.InitiateOffRamp(ctx, ussdadapters.OffRampRequest{
		LoanID:           loanID,
		UserID:           req.UserID,
		AmountUSD:        amountUSD,
		AmountStroops:    req.PrincipalAmount,
		SettlementMethod: "direct",
		IdempotencyKey:   loanID,
	})
	if err != nil {
		// Non-fatal: loan is disbursed, off-ramp can retry.
		a.logger.Warn("off-ramp failed", "loan_id", loanID, "error", err)
	} else {
		a.logger.Info("off-ramp initiated",
			"loan_id", loanID,
			"request_id", offRampResult.RequestID,
			"sequence_id", offRampResult.SequenceID,
			"settlement_method", offRampResult.SettlementMethod,
			"amount_local", offRampResult.AmountLocal,
			"currency", offRampResult.LocalCurrency,
		)

		// Step 7: Update loan with off-ramp details.
		rampProvider := "yellowcard"
		rampFiatAmount := int64(offRampResult.AmountLocal * 100) // cents
		actualMethod := offRampResult.SettlementMethod
		rampDisbStatus := "processing"
		_, updateErr := a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
			RampProvider:       &rampProvider,
			RampRequestID:      &offRampResult.RequestID,
			RampSequenceID:     &offRampResult.SequenceID,
			RampFiatAmount:     &rampFiatAmount,
			RampFiatCurr:       &offRampResult.LocalCurrency,
			SettlementMethod:   &actualMethod,
			DisbursementStatus: &rampDisbStatus,
		})
		if updateErr != nil {
			// Non-fatal.
			a.logger.Warn("failed to record off-ramp details", "loan_id", loanID, "error", updateErr)
		} else {
			a.logger.Info("loan updated with off-ramp details", "loan_id", loanID)
		}
	}

	// Step 8: SMS notification (best-effort).
	if a.notificationSvc != nil {
		loanNumber := ""
		if createResp.LoanNumber != nil {
			loanNumber = *createResp.LoanNumber
		}
		if smsErr := a.notificationSvc.SendLoanRequestConfirmation(ctx, "", loanNumber, amountUSD); smsErr != nil {
			a.logger.Warn("SMS notification failed", "loan_id", loanID, "error", smsErr)
		} else {
			a.logger.Info("SMS notification sent", "loan_id", loanID)
		}
	}

	duration := time.Since(start)
	a.logger.Info("loan disbursement completed",
		"loan_id", loanID,
		"tx_hash", borrowResp.TxHash,
		"total_duration_ms", duration.Milliseconds(),
	)

	// Return as map for USSD handler type assertions.
	totalAmount := req.PrincipalAmount
	if createResp.TotalAmount != nil {
		totalAmount = *createResp.TotalAmount
	}
	return map[string]interface{}{
		"id":           loanID,
		"loan_number":  createResp.LoanNumber,
		"status":       "disbursed",
		"total_amount": totalAmount,
	}, nil
}

// GetUserLoans implements ussd.LoanService.
func (a *LoanServiceAdapter) GetUserLoans(ctx context.Context, userID string) ([]interface{}, error) {
	resp, err := a.loanSvc.GetByUserID(ctx, userID, services.Pagination{Page: 1, PageSize: 20})
	if err != nil {
		a.logger.Error("failed to fetch user loans", "user_id", userID, "error", err)
		return nil, err
	}

	results := make([]interface{}, len(resp.Data))
	for i, l := range resp.Data {
		results[i] = map[string]interface{}{
			"id":           l.ID,
			"loan_number":  l.LoanNumber,
			"status":       l.Status,
			"total_amount": l.TotalAmount,
			"due_date":     l.DueDate,
		}
	}
	a.logger.Info("fetched user loans", "user_id", userID, "count", len(results))
	return results, nil
}

// CheckLoanEligibility implements ussd.LoanService.
func (a *LoanServiceAdapter) CheckLoanEligibility(ctx context.Context, userID string, amount int64, duration int) (*ussd.LoanApproval, error) {
	approved := amount <= a.defaultLimit
	reason := "approved"
	if !approved {
		reason = fmt.Sprintf("Amount exceeds limit of %d stroops", a.defaultLimit)
	}

	a.logger.Info("eligibility check",
		"user_id", userID,
		"amount", amount,
		"approved", approved,
		"reason", reason,
	)

	return &ussd.LoanApproval{
		Approved:     approved,
		Reason:       reason,
		InterestRate: 5.0, // 5% default
	}, nil
}
