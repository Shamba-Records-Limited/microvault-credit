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
	"github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/moneygram"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/yellowcard"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"
)

// DefaultFXBufferPct is the safety margin applied to the re-quoted FX rate
// when persisting entry_rate_used on the loan. 2% is the operating norm
// inherited from the integration plan; override via FXConfig if needed.
const DefaultFXBufferPct = 0.02

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
	offRamps      *offramp.Registry
	loanNotifier  contracts.LoanNotifier
	txnSvc        transaction.Service
	logger        *slog.Logger
	productConfig *ussd.LoanProductConfig
	fxBufferPct   float64
	dedupe        *dedupeGate
}

// FXConfig tunes the entry-rate quoting that happens just before the loan is
// initiated against a provider. BufferPct is applied multiplicatively to the
// quoted buy rate (entry_rate_used = buy_rate * (1 - BufferPct)) and recorded
// on the loan for downstream drift detection.
type FXConfig struct {
	BufferPct float64 // 0.02 = 2 %. Falls back to DefaultFXBufferPct when ≤ 0.
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
	offRamps *offramp.Registry,
	loanNotifier contracts.LoanNotifier,
	txnSvc transaction.Service,
	fxCfg FXConfig,
	logger *slog.Logger,
) (*LoanServiceAdapter, error) {
	if offRamps == nil {
		return nil, fmt.Errorf("offramp registry is required")
	}
	bufferPct := fxCfg.BufferPct
	if bufferPct <= 0 {
		bufferPct = DefaultFXBufferPct
	}
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
		offRamps:      offRamps,
		loanNotifier:  loanNotifier,
		txnSvc:        txnSvc,
		logger:        logger,
		productConfig: cfg,
		fxBufferPct:   bufferPct,
		dedupe:        newDedupeGate(60 * time.Second),
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

	payoutMethod := req.PayoutMethod
	if payoutMethod == "" {
		payoutMethod = offramp.PayoutMethodMobileMoney
	}

	// Dedupe gate — same (user, method, amount) within 60s is treated as a
	// USSD/carrier replay and rejected before any state mutates.
	if !a.dedupe.check(dedupeKey(req.UserID, payoutMethod, req.PrincipalAmount)) {
		a.logger.Warn("duplicate loan request suppressed",
			"user_id", req.UserID,
			"payout_method", payoutMethod,
			"amount_stroops", req.PrincipalAmount,
		)
		return nil, ErrDuplicateLoanRequest
	}

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
		"payout_method", payoutMethod,
	)

	// Build provider-specific options once, ahead of any FX work, so the
	// resolved Provider is the one that will both quote and initiate.
	providerOpts, err := a.buildProviderOptions(payoutMethod, req)
	if err != nil {
		return nil, err
	}

	// Re-quote FX against the chosen provider's Quoter (if it exposes one)
	// and bake the buffer in. The result is recorded on the loan after Create
	// for downstream drift detection; we do not gate creation on a successful
	// quote — providers without a Quoter still need to be initiable.
	entryRate, entryRateSource := a.requoteEntryRate(ctx, providerOpts, req.LocalCurrency)

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

	// Step 2a: Record entry-rate audit fields + requested local amount on
	// the freshly created loan so the poller / refund matcher have the
	// numbers the user saw at quote time.
	a.persistEntryRate(ctx, loanID, entryRate, entryRateSource, req.LocalAmount, req.ChildAccountIndex)

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

	// Step 6: Initiate off-ramp against the resolved provider.
	amountUSD := float64(req.PrincipalAmount) / 1e7
	offrampReq := offramp.Request{
		LoanID:           loanID,
		UserID:           req.UserID,
		RecipientName:    req.RecipientName,
		AmountUSD:        amountUSD,
		AmountStroops:    req.PrincipalAmount,
		DestinationPhone: req.PhoneNumber,
		CountryCode:      req.CountryCode,
		NetworkCode:      req.NetworkCode,
		NetworkName:      req.NetworkName,
		IdempotencyKey:   loanID,
		PayoutMethod:     payoutMethod,
		Options:          providerOpts,
	}
	provider, resolveErr := a.offRamps.Resolve(offrampReq)
	if resolveErr != nil {
		a.logger.Error("off-ramp registry resolve failed",
			"loan_id", loanID,
			"payout_method", payoutMethod,
			"error", resolveErr,
		)
		return nil, fmt.Errorf("resolve off-ramp provider: %w", resolveErr)
	}
	offRampResult, err := provider.Initiate(ctx, offrampReq)
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

		// USDC never left treasury — repay vault immediately.
		repayResp, repayErr := a.stellarSvc.RepayToVault(ctx, stellar.RepayRequest{Amount: borrowResp.AmountBorrowed})
		if repayErr != nil {
			a.logger.Error("CRITICAL: vault repay failed after off-ramp init failure",
				"loan_id", loanID,
				"amount_stroops", borrowResp.AmountBorrowed,
				"error", repayErr,
			)
		} else {
			_, _ = a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{VaultRepayTxHash: &repayResp.TxHash})

			if a.txnSvc != nil {
				repayDesc := "Vault repay after off-ramp init failure"
				txnResp, txnErr := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
					UserID:        &req.UserID,
					LoanID:        &loanID,
					TxType:        models.TxTypeVaultRepay,
					TxCategory:    models.TxCategoryOnChain,
					Amount:        repayResp.AmountRepaid,
					Asset:         "USDC",
					StellarTxHash: &repayResp.TxHash,
					Description:   &repayDesc,
				})
				if txnErr == nil && txnResp != nil {
					s := models.TxStatusSuccess
					_, _ = a.txnSvc.Update(ctx, txnResp.ID, transaction.UpdateTransactionRequest{Status: &s})
				}
			}
			a.logger.Info("vault repaid after off-ramp init failure",
				"loan_id", loanID,
				"repay_tx_hash", repayResp.TxHash,
			)
		}

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
		a.recordSuccessfulInitiate(ctx, loanID, payoutMethod, provider.ID(), offRampResult, req, createResp)
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
		a.logger.Info("loan off-ramp initiated successfully",
			"loan_id", loanID,
			"vault_tx_hash", borrowResp.TxHash,
			"payout_method", payoutMethod,
			"offramp_request_id", offRampResult.RequestID,
			"settlement_method", offRampResult.SettlementMethod,
			"amount_local", offRampResult.AmountLocal,
			"currency", offRampResult.LocalCurrency,
			"total_duration_ms", duration.Milliseconds(),
		)
	}

	// Return as map for USSD handler type assertions. For mobile-money the
	// result is a synchronous disbursement; for cash-pickup it's an
	// awaiting-user state — the poller advances it later.
	totalAmount := req.PrincipalAmount
	if createResp.TotalAmount != nil {
		totalAmount = *createResp.TotalAmount
	}
	status := "disbursed"
	switch {
	case offRampFailed:
		status = "offramp_failed"
	case payoutMethod == offramp.PayoutMethodCashPickup:
		status = "awaiting_user"
	}
	return map[string]interface{}{
		"id":           loanID,
		"loan_number":  createResp.LoanNumber,
		"status":       status,
		"total_amount": totalAmount,
	}, nil
}

// buildProviderOptions assembles the typed Options payload for the chosen
// payout method. MoneyGram requires BirthDate + ChildAccountIndex (its
// adapter rejects nil/wrong-type); YellowCard takes a SettlementMethod.
func (a *LoanServiceAdapter) buildProviderOptions(payoutMethod string, req *ussd.LoanRequest) (offramp.ProviderOptions, error) {
	switch payoutMethod {
	case offramp.PayoutMethodMobileMoney:
		return yellowcard.Options{
			SettlementMethod: yellowcard.SettlementMethodDirect,
		}, nil
	case offramp.PayoutMethodCashPickup:
		if req.BirthDate == "" {
			return nil, fmt.Errorf("cash-pickup requires BirthDate on LoanRequest")
		}
		return moneygram.Options{
			BirthDate:         req.BirthDate,
			ChildAccountIndex: req.ChildAccountIndex,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported payout method %q", payoutMethod)
	}
}

// requoteEntryRate asks the resolved provider's Quoter for a fresh rate and
// applies the configured safety buffer. Returns (rate, source) — source is
// either the provider ID or "fallback" when the provider doesn't implement
// Quoter or the quote fails. The returned rate is always non-negative; a
// zero rate signals "no usable quote" and callers should skip persistence.
func (a *LoanServiceAdapter) requoteEntryRate(ctx context.Context, opts offramp.ProviderOptions, currency string) (float64, string) {
	if currency == "" {
		return 0, ""
	}
	p, err := a.offRamps.Resolve(offramp.Request{Options: opts})
	if err != nil {
		a.logger.Warn("entry-rate re-quote: registry resolve failed", "error", err)
		return 0, ""
	}
	quoter, ok := p.(offramp.Quoter)
	if !ok {
		// Provider doesn't expose Quoter — not an error, just leave entry
		// rate unrecorded.
		return 0, ""
	}
	q, err := quoter.Quote(ctx, offramp.QuoteRequest{Currency: currency})
	if err != nil {
		a.logger.Warn("entry-rate re-quote failed",
			"provider", p.ID(), "currency", currency, "error", err)
		return 0, ""
	}
	rate := q.BuyRate
	if rate == 0 {
		rate = q.Rate
	}
	if rate <= 0 {
		return 0, ""
	}
	buffered := rate * (1.0 - a.fxBufferPct)
	return buffered, string(p.ID())
}

// persistEntryRate writes the entry-rate audit fields and the requested
// local amount + child-account index onto the freshly created loan. Each
// field is optional; nil values are skipped via the UpdateLoanRequest's
// pointer semantics. Errors are logged, not returned — this is audit data,
// not load-bearing for the disbursement path.
func (a *LoanServiceAdapter) persistEntryRate(
	ctx context.Context,
	loanID string,
	entryRate float64,
	entryRateSource string,
	localAmountCents int64,
	childAccountIndex uint32,
) {
	req := loan.UpdateLoanRequest{}
	any := false
	if entryRate > 0 {
		v := entryRate
		req.EntryRateUsed = &v
		any = true
	}
	if entryRateSource != "" {
		v := entryRateSource
		req.EntryRateSource = &v
		any = true
	}
	if a.fxBufferPct > 0 {
		v := a.fxBufferPct
		req.EntryBufferPct = &v
		any = true
	}
	if localAmountCents > 0 {
		v := localAmountCents
		req.RequestedLocalAmount = &v
		any = true
	}
	if childAccountIndex > 0 {
		v := int64(childAccountIndex)
		req.RampChildAccountIndex = &v
		any = true
	}
	if !any {
		return
	}
	if _, err := a.loanSvc.Update(ctx, loanID, req); err != nil {
		a.logger.Warn("failed to persist entry-rate audit fields",
			"loan_id", loanID, "error", err)
	}
}

// recordSuccessfulInitiate persists the off-ramp results onto the loan,
// branching on payout method. YellowCard returns a locked AmountLocal +
// fees that are persisted immediately; MoneyGram returns an interactive
// URL with no locked amount — the poller backfills those later.
func (a *LoanServiceAdapter) recordSuccessfulInitiate(
	ctx context.Context,
	loanID string,
	payoutMethod string,
	providerID offramp.ProviderID,
	result *offramp.Result,
	req *ussd.LoanRequest,
	createResp *loan.LoanResponse,
) {
	rampProvider := string(providerID)
	updateReq := loan.UpdateLoanRequest{
		RampProvider:   &rampProvider,
		RampRequestID:  &result.RequestID,
		RampSequenceID: &result.SequenceID,
	}
	if result.SettlementMethod != "" {
		v := result.SettlementMethod
		updateReq.SettlementMethod = &v
	}

	switch payoutMethod {
	case offramp.PayoutMethodCashPickup:
		// MG returned the interactive URL; persist what's known and let the
		// poller backfill amount_out / external_ref / withdraw_memo.
		mgInitiated := "mg_initiated"
		updateReq.DisbursementStatus = &mgInitiated
		if mg, ok := result.Provider.(moneygram.CashPickupPayload); ok {
			if mg.InteractiveURL != "" {
				v := mg.InteractiveURL
				updateReq.RampInteractiveURL = &v
			}
		}
		if _, err := a.loanSvc.Update(ctx, loanID, updateReq); err != nil {
			a.logger.Warn("failed to record cash-pickup initiation",
				"loan_id", loanID, "error", err)
		}

		// SMS user the interactive URL so they can complete KYC.
		if a.loanNotifier != nil && req.PhoneNumber != "" {
			if mg, ok := result.Provider.(moneygram.CashPickupPayload); ok && mg.InteractiveURL != "" {
				notification := contracts.LoanNotification{
					LoanID:          loanID,
					PhoneNumber:     req.PhoneNumber,
					DisplayAmount:   float64(req.PrincipalAmount) / 1e7,
					DisplayCurrency: "USD",
					InteractiveURL:  mg.InteractiveURL,
				}
				if createResp.LoanNumber != nil {
					notification.LoanNumber = *createResp.LoanNumber
				}
				if smsErr := a.loanNotifier.NotifyLoanCashPickupInitiated(ctx, notification); smsErr != nil {
					a.logger.Warn("cash-pickup SMS failed",
						"loan_id", loanID, "error", smsErr)
				}
			}
		}

	default:
		// Mobile-money: existing YC path — locked AmountLocal + fees.
		rampFiatAmount := int64(result.AmountLocal * 100) // cents
		rampDisbStatus := "processing"
		feeUSD := int64(result.Fee * 100)
		feeLocal := int64(result.FeeLocal * 100)
		updateReq.RampFiatAmount = &rampFiatAmount
		updateReq.RampFiatCurr = &result.LocalCurrency
		updateReq.DisbursementStatus = &rampDisbStatus
		updateReq.RampFeeUSD = &feeUSD
		updateReq.RampFeeLocal = &feeLocal

		// Slippage guard (log-only, no blocking).
		if req.LocalAmount > 0 {
			deviation := (result.AmountLocal - float64(req.LocalAmount)/100.0) / (float64(req.LocalAmount) / 100.0)
			if deviation < -0.02 || deviation > 0.02 {
				a.logger.Warn("SLIPPAGE ALERT: >2% deviation",
					"loan_id", loanID,
					"expected_local", float64(req.LocalAmount)/100.0,
					"actual_local", result.AmountLocal,
					"deviation_pct", deviation*100,
				)
			}
		}

		// Persist conversion data when local currency info is available.
		if req.ConversionRate > 0 {
			disbursementRateBps := int64(req.ConversionRate * 10000)
			updateReq.DisbursementRateBps = &disbursementRateBps
		}
		if req.LocalAmount > 0 {
			updateReq.DisbursementAmtKES = &req.LocalAmount
			totalUSDC := req.PrincipalAmount
			if createResp.TotalAmount != nil {
				totalUSDC = *createResp.TotalAmount
			}
			repaymentKES := int64(float64(totalUSDC) / 1e7 * req.ConversionRate * 100)
			updateReq.RepaymentAmtKES = &repaymentKES
		}

		if _, err := a.loanSvc.Update(ctx, loanID, updateReq); err != nil {
			a.logger.Warn("failed to record off-ramp details", "loan_id", loanID, "error", err)
		} else {
			a.logger.Info("loan updated with off-ramp details", "loan_id", loanID)
		}

		// Record off-ramp transaction.
		if a.txnSvc != nil {
			offRampDesc := fmt.Sprintf("Off-ramp via %s (%s)", providerID, result.SettlementMethod)
			provName := string(providerID)
			if _, txnErr := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
				UserID:           &req.UserID,
				AccountID:        &req.AccountID,
				LoanID:           &loanID,
				TxType:           models.TxTypeOffRamp,
				TxCategory:       models.TxCategoryOffChain,
				Amount:           rampFiatAmount,
				Asset:            result.LocalCurrency,
				ExternalID:       &result.RequestID,
				ExternalProvider: &provName,
				Description:      &offRampDesc,
			}); txnErr != nil {
				a.logger.Warn("failed to record off-ramp transaction", "loan_id", loanID, "error", txnErr)
			}
		}

		// Disbursement SMS is sent by the webhook handler (DisbursementStatusAdapter)
		// when YellowCard confirms completion — not here, to avoid duplicates.
	}
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
