// Package adapters bridges microvault-credit services to microvault's USSD interfaces.
package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	loanproduct "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_product"
	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	ussdadapters "github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd/adapters"
	"github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"
)

// Compile-time check.
var _ ussd.LoanService = (*LoanServiceAdapter)(nil)

// LoanServiceAdapter implements [ussd.LoanService] by orchestrating credit's
// loan service, Stellar vault, YellowCard off-ramp, and SMS notifications.
//
// The active loan product is loaded once at construction and cached in
// productConfig; the USSD handler reads it via [GetProductConfig].
type LoanServiceAdapter struct {
	loanSvc       loan.Service
	productSvc    loanproduct.Service
	stellarSvc    stellar.Service
	offRampSvc    ussdadapters.OffRampService
	loanNotifier  contracts.LoanNotifier
	txnSvc        transaction.Service
	logger        *slog.Logger
	productConfig *ussd.LoanProductConfig
}

// NewLoanServiceAdapter creates a new [LoanServiceAdapter].
//
// It loads the highest-priority active loan product from the database and
// caches its configuration. Returns an error if no active product is found.
func NewLoanServiceAdapter(
	ctx context.Context,
	loanSvc loan.Service,
	productSvc loanproduct.Service,
	stellarSvc stellar.Service,
	offRampSvc ussdadapters.OffRampService,
	loanNotifier contracts.LoanNotifier,
	txnSvc transaction.Service,
	logger *slog.Logger,
) (*LoanServiceAdapter, error) {
	// Load the highest-priority active product (ordered by priority_order ASC).
	products, err := productSvc.GetActive(ctx, services.Pagination{Page: 1, PageSize: 1})
	if err != nil {
		return nil, fmt.Errorf("load active loan product: %w", err)
	}
	if len(products.Data) == 0 {
		return nil, fmt.Errorf("no active loan product found; run the 000006 migration to seed one")
	}
	p := products.Data[0]

	schedule := "lump_sum"
	if len(p.AllowedRepaymentSchedules) > 0 {
		schedule = p.AllowedRepaymentSchedules[0]
	}

	cfg := &ussd.LoanProductConfig{
		ProductID:         p.ID,
		MinAmountCents:    p.MinAmount,
		MaxAmountCents:    p.MaxAmount,
		Currency:          p.Currency,
		DurationDays:      p.MinDurationDays,
		RepaymentSchedule: schedule,
		InterestRateBps:   p.InterestRateBps,
	}

	logger.Info("loan product loaded",
		"product_id", p.ID,
		"name", p.Name,
		"currency", p.Currency,
		"min_amount_cents", p.MinAmount,
		"max_amount_cents", p.MaxAmount,
		"duration_days", p.MinDurationDays,
	)

	return &LoanServiceAdapter{
		loanSvc:       loanSvc,
		productSvc:    productSvc,
		stellarSvc:    stellarSvc,
		offRampSvc:    offRampSvc,
		loanNotifier:  loanNotifier,
		txnSvc:        txnSvc,
		logger:        logger,
		productConfig: cfg,
	}, nil
}

// GetProductConfig returns the cached loan product configuration.
// Returns nil if no product was loaded (should not happen after successful construction).
func (a *LoanServiceAdapter) GetProductConfig() *ussd.LoanProductConfig {
	return a.productConfig
}

// RequestLoan implements ussd.LoanService. It orchestrates the full loan
// disbursement cycle: eligibility → create → approve → vault borrow → disburse
// → off-ramp → notify.
func (a *LoanServiceAdapter) RequestLoan(ctx context.Context, req *ussd.LoanRequest) (interface{}, error) {
	start := time.Now()

	// Amount validation is handled by the USSD handler against the loan product
	// config (fiat-denominated limits). By this point the request is pre-approved.

	// Use KES for notifications when local amount is available.
	notifyAmount := float64(req.PrincipalAmount) / 1e7 // fallback USD
	notifyCurrency := "USD"
	if req.LocalAmount > 0 {
		notifyAmount = float64(req.LocalAmount) / 100.0
		notifyCurrency = req.LocalCurrency
	}

	a.logger.Info("loan request received",
		"user_id", req.UserID,
		"product_id", req.ProductID,
		"amount_stroops", req.PrincipalAmount,
		"local_amount_cents", req.LocalAmount,
		"currency", req.LocalCurrency,
	)

	// Step 1: Query dynamic vault APR; fall back to product rate.
	interestRateBps := a.productConfig.InterestRateBps
	aprWad, err := a.stellarSvc.GetBorrowAPR(ctx)
	if err != nil {
		a.logger.Warn("failed to fetch vault APR, using fallback", "error", err)
	} else if aprWad > 0 {
		interestRateBps = int32(aprWad / 1e14) // WAD (1e18) → bps (1e4)
	}

	// Step 2: Create loan record.
	productID := req.ProductID
	createResp, err := a.loanSvc.Create(ctx, loan.CreateLoanRequest{
		UserID:            req.UserID,
		AccountID:         req.AccountID,
		ProductID:         &productID,
		PrincipalAmount:   req.PrincipalAmount,
		PrincipalAsset:    req.PrincipalAsset,
		InterestRateBps:   interestRateBps,
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
	// Use the nil UUID for system auto-approvals (approved_by is UUID in the DB).
	_, err = a.loanSvc.Approve(ctx, loanID, loan.ApproveLoanRequest{ApprovedBy: "00000000-0000-0000-0000-000000000000"})
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

	// Step 4: Borrow from Stellar vault.
	borrowResp, err := a.stellarSvc.BorrowFromVault(ctx, stellar.BorrowRequest{
		RecipientAddress: req.StellarAddress,
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

	// Step 6: Initiate off-ramp.
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
	offRampFailed := err != nil
	if offRampFailed {
		a.logger.Error("off-ramp failed — vault borrow succeeded, USDC in treasury, fiat not disbursed",
			"loan_id", loanID,
			"vault_tx_hash", borrowResp.TxHash,
			"error", err,
		)
		// Mark as offramp_failed so a retry mechanism can pick it up.
		offRampFailedStatus := "offramp_failed"
		_, _ = a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
			DisbursementStatus: &offRampFailedStatus,
		})

		// Notify user of failure (best-effort).
		if a.loanNotifier != nil && req.PhoneNumber != "" {
			loanNumber := ""
			if createResp.LoanNumber != nil {
				loanNumber = *createResp.LoanNumber
			}
			_ = a.loanNotifier.NotifyLoanFailed(ctx, contracts.LoanNotification{
				LoanID:          loanID,
				LoanNumber:      loanNumber,
				PhoneNumber:     req.PhoneNumber,
				DisplayAmount:   notifyAmount,
				DisplayCurrency: notifyCurrency,
			})
		}
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

		// Step 7: Update loan with off-ramp details, fees, and conversion data.
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

		// Disbursement SMS is sent by the webhook handler (DisbursementStatusAdapter)
		// when YellowCard confirms completion — not here, to avoid duplicates.
	}

	duration := time.Since(start)
	if offRampFailed {
		a.logger.Warn("loan disbursement partial — vault borrow ok, off-ramp failed (retryable)",
			"loan_id", loanID,
			"vault_tx_hash", borrowResp.TxHash,
			"disbursement_status", "offramp_failed",
			"total_duration_ms", duration.Milliseconds(),
		)
	} else {
		a.logger.Info("loan disbursement completed successfully",
			"loan_id", loanID,
			"vault_tx_hash", borrowResp.TxHash,
			"offramp_request_id", offRampResult.RequestID,
			"settlement_method", offRampResult.SettlementMethod,
			"amount_local", offRampResult.AmountLocal,
			"currency", offRampResult.LocalCurrency,
			"total_duration_ms", duration.Milliseconds(),
		)
	}

	// Return as map for USSD handler type assertions.
	totalAmount := req.PrincipalAmount
	if createResp.TotalAmount != nil {
		totalAmount = *createResp.TotalAmount
	}
	status := "disbursed"
	if offRampFailed {
		status = "offramp_failed"
	}
	return map[string]interface{}{
		"id":           loanID,
		"loan_number":  createResp.LoanNumber,
		"status":       status,
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
//
// Fiat-denominated limit checks are performed by the USSD handler using
// [GetProductConfig]. This method fetches the dynamic vault APR and always
// approves the request (the amount has already been validated).
func (a *LoanServiceAdapter) CheckLoanEligibility(ctx context.Context, userID string, amount int64, duration int) (*ussd.LoanApproval, error) {
	// Fetch dynamic APR from vault; fall back to product rate.
	fallbackRate := float64(a.productConfig.InterestRateBps) / 10000.0 // bps → decimal
	interestRate := fallbackRate
	aprWad, err := a.stellarSvc.GetBorrowAPR(ctx)
	if err != nil {
		a.logger.Warn("failed to fetch vault APR for eligibility, using fallback", "error", err)
	} else if aprWad > 0 {
		interestRate = float64(aprWad) / 1e18 // WAD → decimal (e.g. 0.08 for 8%)
	}

	a.logger.Info("eligibility check",
		"user_id", userID,
		"amount", amount,
		"approved", true,
		"interest_rate", interestRate,
	)

	return &ussd.LoanApproval{
		Approved:     true,
		Reason:       "approved",
		InterestRate: interestRate * 100, // decimal → percentage (e.g. 8.0 for 8%)
	}, nil
}
