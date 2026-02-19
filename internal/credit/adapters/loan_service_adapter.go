// Package adapters bridges microvault-credit services to microvault's USSD interfaces.
package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	"github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	ussdadapters "github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd/adapters"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"
)

// Compile-time check.
var _ ussd.LoanService = (*LoanServiceAdapter)(nil)

// LoanServiceAdapter implements ussd.LoanService by orchestrating credit's loan
// service, Stellar vault, YellowCard off-ramp, and SMS notifications.
type LoanServiceAdapter struct {
	loanSvc      loan.Service
	stellarSvc   stellar.Service
	offRampSvc   ussdadapters.OffRampService
	loanNotifier contracts.LoanNotifier
	txnSvc       transaction.Service
	logger       *slog.Logger
	defaultLimit int64 // auto-approve limit in stroops
}

// NewLoanServiceAdapter creates a new LoanServiceAdapter.
func NewLoanServiceAdapter(
	loanSvc loan.Service,
	stellarSvc stellar.Service,
	offRampSvc ussdadapters.OffRampService,
	loanNotifier contracts.LoanNotifier,
	txnSvc transaction.Service,
	logger *slog.Logger,
	defaultLimit int64,
) *LoanServiceAdapter {
	return &LoanServiceAdapter{
		loanSvc:      loanSvc,
		stellarSvc:   stellarSvc,
		offRampSvc:   offRampSvc,
		loanNotifier: loanNotifier,
		txnSvc:       txnSvc,
		logger:       logger,
		defaultLimit: defaultLimit,
	}
}

// RequestLoan implements ussd.LoanService. It orchestrates the full loan
// disbursement cycle: eligibility → create → approve → vault borrow → disburse
// → off-ramp → notify.
func (a *LoanServiceAdapter) RequestLoan(ctx context.Context, req *ussd.LoanRequest) (interface{}, error) {
	start := time.Now()

	// Step 1: Auto-approve eligibility check.
	// Use KES for notifications when local amount is available
	notifyAmount := float64(req.PrincipalAmount) / 1e7 // fallback USD
	notifyCurrency := "USD"
	if req.LocalAmount > 0 {
		notifyAmount = float64(req.LocalAmount) / 100.0
		notifyCurrency = req.LocalCurrency
	}

	if req.PrincipalAmount > a.defaultLimit {
		reason := "Loan amount exceeds the current limit"
		a.logger.Warn("loan rejected: exceeds limit",
			"user_id", req.UserID,
			"amount", req.PrincipalAmount,
			"limit", a.defaultLimit,
		)

		// Notify user of rejection (best-effort).
		if a.loanNotifier != nil && req.PhoneNumber != "" {
			if smsErr := a.loanNotifier.NotifyLoanRejected(ctx, contracts.LoanNotification{
				PhoneNumber:     req.PhoneNumber,
				DisplayAmount:   notifyAmount,
				DisplayCurrency: notifyCurrency,
				Reason:          reason,
			}); smsErr != nil {
				a.logger.Warn("rejection SMS failed", "user_id", req.UserID, "error", smsErr)
			}
		}

		return &ussd.LoanApproval{
			Approved: false,
			Reason:   reason,
		}, nil
	}
	a.logger.Info("loan eligibility check",
		"user_id", req.UserID,
		"amount", req.PrincipalAmount,
		"limit", a.defaultLimit,
		"approved", true,
	)

	// Step 2: Query dynamic vault APR instead of hardcoded rate.
	interestRateBps := int32(500) // fallback 5%
	aprWad, err := a.stellarSvc.GetBorrowAPR(ctx)
	if err != nil {
		a.logger.Warn("failed to fetch vault APR, using fallback", "error", err)
	} else if aprWad > 0 {
		interestRateBps = int32(aprWad / 1e14) // WAD (1e18) → bps (1e4)
	}

	// Step 3: Create loan record.
	createResp, err := a.loanSvc.Create(ctx, loan.CreateLoanRequest{
		UserID:            req.UserID,
		AccountID:         req.AccountID,
		PrincipalAmount:   req.PrincipalAmount,
		PrincipalAsset:    req.PrincipalAsset,
		InterestRateBps:   interestRateBps,
		DurationDays:      req.DurationDays,
		RepaymentSchedule: req.RepaymentSched,
	})
	_ = notifyCurrency // used in logging context
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

	// Step 4: Auto-approve.
	_, err = a.loanSvc.Approve(ctx, loanID, loan.ApproveLoanRequest{ApprovedBy: "system"})
	if err != nil {
		a.logger.Error("failed to approve loan", "loan_id", loanID, "error", err)
		return nil, fmt.Errorf("approve loan: %w", err)
	}
	a.logger.Info("loan auto-approved", "loan_id", loanID)

	// Notify user of approval (best-effort) — use KES amount when available.
	if a.loanNotifier != nil && req.PhoneNumber != "" {
		loanNumber := ""
		if createResp.LoanNumber != nil {
			loanNumber = *createResp.LoanNumber
		}
		if smsErr := a.loanNotifier.NotifyLoanApproved(ctx, contracts.LoanNotification{
			LoanID:          loanID,
			LoanNumber:      loanNumber,
			PhoneNumber:     req.PhoneNumber,
			DisplayAmount:   notifyAmount,
			DisplayCurrency: notifyCurrency,
		}); smsErr != nil {
			a.logger.Warn("approval SMS failed", "loan_id", loanID, "error", smsErr)
		}
	}

	// Step 5: Borrow from Stellar vault.
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
		"borrow_index", borrowResp.BorrowIndex,
	)

	// Capture borrow index at origination.
	if borrowResp.BorrowIndex > 0 {
		_, _ = a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
			BorrowIndex: &borrowResp.BorrowIndex,
		})
	}

	// Record vault borrow transaction.
	if a.txnSvc != nil {
		vaultDesc := "USDC vault borrow for loan disbursement"
		txnResp, txnErr := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
			UserID:        &req.UserID,
			AccountID:     &req.AccountID,
			LoanID:        &loanID,
			TxType:        models.TxTypeVaultBorrow,
			TxCategory:    models.TxCategoryOnChain,
			Amount:        borrowResp.AmountBorrowed,
			Asset:         "USDC",
			StellarTxHash: &borrowResp.TxHash,
			Description:   &vaultDesc,
		})
		if txnErr != nil {
			a.logger.Warn("failed to record vault borrow transaction", "loan_id", loanID, "error", txnErr)
		} else if txnResp != nil {
			// Transition: pending -> submitted -> success (vault TX is already confirmed).
			submittedStatus := models.TxStatusSubmitted
			_, _ = a.txnSvc.Update(ctx, txnResp.ID, transaction.UpdateTransactionRequest{
				Status: &submittedStatus,
			})
			successStatus := models.TxStatusSuccess
			_, _ = a.txnSvc.Update(ctx, txnResp.ID, transaction.UpdateTransactionRequest{
				Status: &successStatus,
			})
		}
	}

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

	// Step 7: Initiate off-ramp.
	amountUSD := float64(req.PrincipalAmount) / 1e7
	offRampResult, err := a.offRampSvc.InitiateOffRamp(ctx, ussdadapters.OffRampRequest{
		LoanID:           loanID,
		UserID:           req.UserID,
		RecipientName:    req.RecipientName,
		AmountUSD:        amountUSD,
		AmountStroops:    req.PrincipalAmount,
		DestinationPhone: req.PhoneNumber,
		CountryCode:      req.CountryCode,
		NetworkCode:      req.NetworkCode,
		NetworkName:      req.NetworkName,
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

		// Slippage guard (log-only, no blocking).
		if req.LocalAmount > 0 {
			deviation := (offRampResult.AmountLocal - float64(req.LocalAmount)/100.0) / (float64(req.LocalAmount) / 100.0)
			if deviation < -0.02 || deviation > 0.02 {
				a.logger.Warn("SLIPPAGE ALERT: >2% deviation",
					"loan_id", loanID,
					"expected_local", float64(req.LocalAmount)/100.0,
					"actual_local", offRampResult.AmountLocal,
					"deviation_pct", deviation*100,
				)
			}
		}

		// Step 8: Update loan with off-ramp details, fees, and conversion data.
		rampProvider := "yellowcard"
		rampFiatAmount := int64(offRampResult.AmountLocal * 100) // cents
		actualMethod := offRampResult.SettlementMethod
		rampDisbStatus := "processing"
		feeUSD := int64(offRampResult.Fee * 100)       // USD cents
		feeLocal := int64(offRampResult.FeeLocal * 100) // KES cents

		updateReq := loan.UpdateLoanRequest{
			RampProvider:       &rampProvider,
			RampRequestID:      &offRampResult.RequestID,
			RampSequenceID:     &offRampResult.SequenceID,
			RampFiatAmount:     &rampFiatAmount,
			RampFiatCurr:       &offRampResult.LocalCurrency,
			SettlementMethod:   &actualMethod,
			DisbursementStatus: &rampDisbStatus,
			RampFeeUSD:         &feeUSD,
			RampFeeLocal:       &feeLocal,
		}

		// Persist conversion data when local currency info is available.
		if req.ConversionRate > 0 {
			disbursementRateBps := int64(req.ConversionRate * 10000)
			updateReq.DisbursementRateBps = &disbursementRateBps
		}
		if req.LocalAmount > 0 {
			updateReq.DisbursementAmtKES = &req.LocalAmount
			// Indicative repayment in KES: total USDC owed → KES at current rate.
			totalUSDC := req.PrincipalAmount
			if createResp.TotalAmount != nil {
				totalUSDC = *createResp.TotalAmount
			}
			repaymentKES := int64(float64(totalUSDC) / 1e7 * req.ConversionRate * 100)
			updateReq.RepaymentAmtKES = &repaymentKES
		}

		_, updateErr := a.loanSvc.Update(ctx, loanID, updateReq)
		if updateErr != nil {
			// Non-fatal.
			a.logger.Warn("failed to record off-ramp details", "loan_id", loanID, "error", updateErr)
		} else {
			a.logger.Info("loan updated with off-ramp details", "loan_id", loanID)
		}

		// Record off-ramp transaction.
		if a.txnSvc != nil {
			offRampDesc := fmt.Sprintf("Off-ramp via YellowCard (%s)", offRampResult.SettlementMethod)
			provider := "yellowcard"
			_, txnErr := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
				UserID:           &req.UserID,
				AccountID:        &req.AccountID,
				LoanID:           &loanID,
				TxType:           models.TxTypeOffRamp,
				TxCategory:       models.TxCategoryOffChain,
				Amount:           rampFiatAmount,
				Asset:            offRampResult.LocalCurrency,
				ExternalID:       &offRampResult.RequestID,
				ExternalProvider: &provider,
				Description:      &offRampDesc,
			})
			if txnErr != nil {
				a.logger.Warn("failed to record off-ramp transaction", "loan_id", loanID, "error", txnErr)
			}
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
			"id":                     l.ID,
			"loan_number":            l.LoanNumber,
			"status":                 l.Status,
			"total_amount":           l.TotalAmount,
			"due_date":               l.DueDate,
			"disbursement_amount_kes": l.DisbursementAmtKES,
			"repayment_amount_kes":   l.RepaymentAmtKES,
			"borrow_index":           l.BorrowIndex,
			"ramp_fee_usd":           l.RampFeeUSD,
			"ramp_fee_local":         l.RampFeeLocal,
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

	// Fetch dynamic APR from vault.
	interestRate := 0.05 // 5% fallback
	aprWad, err := a.stellarSvc.GetBorrowAPR(ctx)
	if err == nil && aprWad > 0 {
		interestRate = float64(aprWad) / 1e18 // WAD → decimal (e.g. 0.08 for 8%)
	}

	a.logger.Info("eligibility check",
		"user_id", userID,
		"amount", amount,
		"approved", approved,
		"reason", reason,
		"interest_rate", interestRate,
	)

	return &ussd.LoanApproval{
		Approved:     approved,
		Reason:       reason,
		InterestRate: interestRate * 100, // decimal → percentage (e.g. 8.0 for 8%)
	}, nil
}
