// Package adapters bridges microvault-credit services to microvault's USSD interfaces.
package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/samber/oops"

	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"

	creditmodels "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	loanproduct "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_product"
	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	"github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/cashin"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/moneygram"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/relay"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/stellaranchor"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/yellowcard"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"

	"github.com/Shamba-Records-Limited/microvault/pkg/urlshortener"
)

// DefaultFXBufferPct is the safety margin applied to the re-quoted FX rate
// when persisting entry_rate_used on the loan. 2% is the operating norm
// inherited from the integration plan; override via FXConfig if needed.
const DefaultFXBufferPct = 0.02

// centStroops is one USDC cent in stroops. Cash-out anchors (MoneyGram,
// mobile-money partners) quote and reconcile amounts at 2 decimal places, so
// every cash-out principal is rounded to a whole cent before it is stored,
// borrowed, or sent on-chain.
const centStroops int64 = 100_000

// roundToCentStroops rounds a stroop amount to the nearest whole USDC cent
// (round-half-up), using integer math only. A positive sub-cent amount never
// rounds down to zero.
func roundToCentStroops(stroops int64) int64 {
	if stroops <= 0 {
		return stroops
	}
	rounded := (stroops + centStroops/2) / centStroops * centStroops
	if rounded == 0 {
		return centStroops
	}
	return rounded
}

// Compile-time check.
var _ ussd.LoanService = (*LoanServiceAdapter)(nil)

// LoanServiceAdapter implements [ussd.LoanService] by orchestrating credit's
// loan service, Stellar vault, YellowCard off-ramp, and SMS notifications.
//
// The active loan product is loaded once at construction and cached in
// productConfig; the USSD handler reads it via [GetProductConfig].
type LoanServiceAdapter struct {
	loanSvc        loan.Service
	productSvc     loanproduct.Service
	stellarSvc     stellar.Service
	offRamps       *offramp.Registry
	relayRouter    *relay.Router
	loanNotifier   contracts.LoanNotifier
	txnSvc         transaction.Service
	logger         *slog.Logger
	productConfig  *ussd.LoanProductConfig
	fxBuffer       offramp.RateBuffer
	dedupe         *dedupeGate
	fxOrch         *moneygram.FXOrchestrator // optional; wired post-construction
	publicBaseURL  string                    // origin for SMS short-links; optional
	shortener      urlshortener.Shortener    // optional; further shortens the SMS link
	accountEnsurer AccountEnsurer            // ensures the child on-chain identity exists before lending

	repayAnchor         *stellaranchor.Client // memo-scoped SEP-24 client for borrower cash deposits
	repayTreasuryPubkey string                // deposit destination
	repayAuthPubkey     string                // SEP-10 signer, and the child-memo namespace
	repayWindow         time.Duration         // how long an opened deposit stays valid
	cashIn              *cashin.Registry      // prompt-capable collection providers; nil disables prompting
}

// repaymentWindow is how long a borrower has to complete a cash deposit.
//
// Days, not minutes: committing in the webview and then walking to an agent
// over the following days is the normal case, not an edge one.
func (a *LoanServiceAdapter) repaymentWindow() time.Duration {
	if a.repayWindow > 0 {
		return a.repayWindow
	}
	return defaultRepaymentWindow
}

// defaultRepaymentWindow matches the deposit poller's expiry expectations.
const defaultRepaymentWindow = 96 * time.Hour

// repaymentStatusInitiated mirrors models.LoanRepaymentStatusInitiated. Named
// here so this file does not import the model package for one constant.
const repaymentStatusInitiated = "initiated"

// sendRepaymentLink delivers the interactive URL by SMS. Best effort: the
// deposit and quote lock stand whether or not the SMS lands.
func (a *LoanServiceAdapter) sendRepaymentLink(ctx context.Context, loanRow *loan.LoanResponse, phoneNumber, interactiveURL string, quote *ussd.RepaymentQuote, expiresAt time.Time) {
	if a.loanNotifier == nil {
		a.logger.Warn("no loan notifier wired; repayment link not delivered",
			"loan_id", loanRow.ID)
		return
	}

	// Same ladder as the cash-pickup link: dub first, and mint a /r/{code}
	// bearer redirect only when dub produced nothing.
	link, err := shortenedLink(ctx, a.shortener, interactiveURL, "")
	if err != nil {
		a.logger.Warn("shorten failed for repayment link; falling back",
			"loan_id", loanRow.ID, "error", err)
	}
	if link == "" {
		link = interactiveURL
		if a.publicBaseURL != "" {
			link = a.mintRedirectLink(ctx, loanRow.ID, interactiveURL)
		}
	}

	displayAmount := float64(quote.AmountUSDCStroops) / 1e7
	displayCurrency := "USDC"
	if quote.AmountLocalCents > 0 && quote.LocalCurrency != "" {
		displayAmount = float64(quote.AmountLocalCents) / 100.0
		displayCurrency = quote.LocalCurrency
	}

	n := contracts.LoanNotification{
		LoanID:      loanRow.ID,
		UserID:      loanRow.UserID,
		PhoneNumber: phoneNumber,
		Amount:      quote.AmountUSDCStroops,
		// Local currency, matching what the USSD screen quoted. The borrower
		// works from one figure; the deposit settles in USDC regardless, and
		// MoneyGram converts at its own counter rate either way.
		DisplayAmount:   displayAmount,
		DisplayCurrency: displayCurrency,
		InteractiveURL:  link,
		// The copy quotes how long the borrower has; without this it falls
		// back to a vague "soon".
		RepaymentExpiresAt: &expiresAt,
	}
	if loanRow.LoanReference != nil {
		n.LoanReference = *loanRow.LoanReference
	}

	if err := a.loanNotifier.NotifyRepaymentInitiated(ctx, n); err != nil {
		a.logger.Error("failed to send repayment link",
			"loan_id", loanRow.ID, "error", err)
	}
}

// AccountEnsurer guarantees a user's child Stellar account exists on-chain
// before a loan is disbursed. Implemented by the core user-service adapter,
// which holds the wallet derivation seed.
type AccountEnsurer interface {
	EnsureOnChainAccount(ctx context.Context, accountIndex int, address string) error
}

// notifyLeakGuard catches a notifier that fails to self-bound. Sized well
// above any real retry sequence so it never cancels one mid-flight.
const notifyLeakGuard = 10 * time.Minute

// notifyAsync sends a borrower notification off the disbursement path, on a
// detached context so it outlives a cancelled pipeline.
func (a *LoanServiceAdapter) notifyAsync(label, loanID string, send func(ctx context.Context) error) {
	if a.loanNotifier == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), notifyLeakGuard)
		defer cancel()
		if err := send(ctx); err != nil {
			a.logger.Warn("borrower notification failed", "notification", label, "loan_id", loanID, "error", err)
		}
	}()
}

// InitiateRepayment freezes the payoff in USDC and opens a MoneyGram cash
// deposit under the borrower's own SEP-10 memo.
func (a *LoanServiceAdapter) InitiateRepayment(ctx context.Context, loanID, phoneNumber string) error {
	errb := oops.In(pkgErrors.DomainRepaymentCashIn).Tags("moneygram", "sep24").With(pkgErrors.AttrLoanID, loanID)

	// Synchronous, because a USSD screen that promises an SMS must not be shown
	// when the request could never have produced one. Both checks are local.
	if a.repayAnchor == nil || a.repayTreasuryPubkey == "" || a.repayAuthPubkey == "" {
		return errb.Code(pkgErrors.CodeAnchorNotWired).Errorf("repayment anchor is not configured")
	}
	if loanID == "" || phoneNumber == "" {
		return errb.Code(pkgErrors.CodeMissingAccount).Errorf("loan id and phone number are both required")
	}

	// Everything past here talks to MoneyGram and took over fifteen seconds
	// against the sandbox — past the point Africa's Talking abandons the
	// session. It runs on its own context so it outlives the USSD turn.
	go a.runRepaymentInitiation(loanID, phoneNumber)

	return nil
}

// routeMobileMoney returns a payout-method alias to pin, or "" to leave
// dispatch alone. Mobile money only — cash pickup is the borrower's choice.
func (a *LoanServiceAdapter) routeMobileMoney(ctx context.Context, req offramp.Request, amountUSD float64) string {
	if a.relayRouter == nil || !a.relayRouter.Enabled() {
		return ""
	}
	if req.PayoutMethod != offramp.PayoutMethodMobileMoney || req.Options != nil {
		return ""
	}

	quote, err := a.relayRouter.Best(ctx, relay.RateRequest{
		Direction:    relay.DirectionOffRamp,
		FiatCurrency: offramp.LocalCurrency(req.CountryCode),
		CountryCode:  req.CountryCode,
		CryptoAmount: amountUSD,
	})
	if err != nil {
		a.logger.Warn("relay could not route this disbursement, using the default provider",
			pkgErrors.AttrLoanID, req.LoanID, "error", err)
		return ""
	}

	alias := payoutAliasFor(quote.Provider)
	if alias == "" {
		a.logger.Warn("relay picked a provider with no payout alias, using the default",
			pkgErrors.AttrLoanID, req.LoanID, pkgErrors.AttrProvider, quote.Provider)
		return ""
	}

	a.logger.Info("relay routed disbursement",
		pkgErrors.AttrLoanID, req.LoanID,
		pkgErrors.AttrProvider, quote.Provider,
		"effective_rate", quote.EffectiveRate,
		"amount_usd", amountUSD)
	return alias
}

// payoutAliasFor maps a routed provider onto the payout method the off-ramp
// registry resolves it by.
func payoutAliasFor(provider string) string {
	switch offramp.ProviderID(provider) {
	case offramp.ProviderYellowCard:
		return offramp.PayoutMethodMobileMoney
	case offramp.ProviderFonbnk:
		return PayoutMethodFonbnkMobileMoney
	}
	return ""
}

// PayoutMethodFonbnkMobileMoney is the off-ramp registry alias Fonbnk is
// registered under. Distinct from offramp.PayoutMethodMobileMoney so the
// unrouted default keeps resolving to YellowCard.
const PayoutMethodFonbnkMobileMoney = "mobile_money_fonbnk"

// depositMemoFor is the memo MoneyGram stamps on the inbound payment: the loan
// reference, else the loan ID, truncated to MEMO_TEXT's 28 bytes.
func depositMemoFor(loanRow *loan.LoanResponse, loanID string) string {
	memo := loanID
	if loanRow.LoanReference != nil && *loanRow.LoanReference != "" {
		memo = *loanRow.LoanReference
	}
	if len(memo) > 28 {
		memo = memo[:28]
	}
	return memo
}

// depositCorridorErr rejects a payoff outside MoneyGram's cash-in corridor.
// The two ends carry different codes because they are actioned differently.
func depositCorridorErr(errb oops.OopsErrorBuilder, payoffStroops int64) error {
	switch {
	case payoffStroops < ussd.MinMoneyGramDepositStroops:
		return errb.
			Code(pkgErrors.CodeBelowAnchorMinimum).
			With("payoff_stroops", payoffStroops).
			With("minimum_stroops", ussd.MinMoneyGramDepositStroops).
			Errorf("payoff is below MoneyGram's deposit floor")
	case payoffStroops > ussd.MaxMoneyGramDepositStroops:
		return errb.
			Code(pkgErrors.CodeAboveAnchorMaximum).
			With("payoff_stroops", payoffStroops).
			With("maximum_stroops", ussd.MaxMoneyGramDepositStroops).
			Errorf("payoff is above MoneyGram's deposit ceiling")
	}
	return nil
}

// runRepaymentInitiation does the slow half of opening a cash deposit. Every
// exit path SMSes the outcome — the USSD session is already gone.
func (a *LoanServiceAdapter) runRepaymentInitiation(loanID, phoneNumber string) {
	ctx, cancel := context.WithTimeout(context.Background(), repaymentInitiationTimeout)
	defer cancel()

	errb := oops.In(pkgErrors.DomainRepaymentCashIn).Tags("moneygram", "sep24").With(pkgErrors.AttrLoanID, loanID)

	fail := func(err error) {
		a.logger.Error("repayment initiation failed",
			pkgErrors.AttrLoanID, loanID, "error", err)
		a.sendRepaymentFailed(ctx, loanID, phoneNumber)
	}

	quote, err := a.GetRepaymentQuote(ctx, loanID)
	if err != nil {
		fail(errb.Code(pkgErrors.CodeQuoteFailed).Wrapf(err, "could not quote the payoff"))
		return
	}
	if err := depositCorridorErr(errb, quote.AmountUSDCStroops); err != nil {
		fail(err)
		return
	}

	loanRow, err := a.loanSvc.GetByID(ctx, loanID)
	if err != nil {
		fail(errb.Code(pkgErrors.CodeLoanLoadFailed).Wrapf(err, "could not load the loan"))
		return
	}
	if loanRow.RampChildAccountIndex == nil {
		fail(errb.Code(pkgErrors.CodeMissingAccountIndex).Errorf("loan has no child account index to scope the SEP-10 session"))
		return
	}

	// Derived from the auth wallet, not the treasury. ChildAccountMemo is
	// namespaced by the key it is seeded with, and the poller seeds it with
	// the anchor client's auth address — seeding it differently here would put
	// the deposit in a memo space the poller never queries, so the borrower
	// could pay and nothing would ever see it.
	childMemo := stellaranchor.ChildAccountMemo(a.repayAuthPubkey, uint32(*loanRow.RampChildAccountIndex))

	resp, err := a.repayAnchor.InitiateDeposit(ctx, childMemo, stellaranchor.DepositRequest{
		AssetCode: "USDC",
		Amount:    fmt.Sprintf("%.2f", float64(quote.AmountUSDCStroops)/1e7),
		Lang:      "en",
		// Destination is the treasury: child accounts hold no USDC trustline,
		// and adding one per borrower would cost a sponsored reserve for an
		// account that only ever passes funds through.
		Account: a.repayTreasuryPubkey,
		// SEP-10's memo identifies the borrower; this identifies which loan.
		// Optional in the spec — deposit_memo reports what was attached.
		Memo:     depositMemoFor(loanRow, loanID),
		MemoType: "text",
	})
	if err != nil {
		fail(errb.Code(pkgErrors.CodeDepositInitFailed).With("child_memo", childMemo).Wrapf(err, "anchor refused the deposit"))
		return
	}

	now := time.Now()
	expiresAt := now.Add(a.repaymentWindow())
	status := repaymentStatusInitiated
	if _, err := a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RepaymentStatus:        &status,
		RepaymentPayoffStroops: &quote.AmountUSDCStroops,
		RepaymentLockedAt:      &now,
		RepaymentExpiresAt:     &expiresAt,
		RepaymentMGTxID:        &resp.ID,
	}); err != nil {
		// The deposit exists at MoneyGram but we have no record of it, so the
		// poller will never drive it and a borrower who pays is unreconciled.
		// Louder than the other failures for that reason.
		a.logger.Error("CRITICAL: deposit opened but the quote lock was not recorded",
			pkgErrors.AttrLoanID, loanID,
			pkgErrors.AttrMoneyGramTxID, resp.ID,
			"error", err)
		a.sendRepaymentFailed(ctx, loanID, phoneNumber)
		return
	}

	a.sendRepaymentLink(ctx, loanRow, phoneNumber, resp.URL, quote, expiresAt)
}

// repaymentInitiationTimeout bounds the background initiation. Generous
// because it spans two MoneyGram round trips and a link shortener, but finite
// so a hung provider cannot leak a goroutine per repayment attempt.
const repaymentInitiationTimeout = 2 * time.Minute

// sendRepaymentFailed tells the borrower no deposit was opened.
//
// Best effort, and deliberately vague: the borrower can act on "try again",
// not on which leg of the anchor handshake failed.
func (a *LoanServiceAdapter) sendRepaymentFailed(ctx context.Context, loanID, phoneNumber string) {
	if a.loanNotifier == nil || phoneNumber == "" {
		return
	}
	n := contracts.LoanNotification{
		LoanID:      loanID,
		PhoneNumber: phoneNumber,
	}
	if loanRow, err := a.loanSvc.GetByID(ctx, loanID); err == nil && loanRow.LoanReference != nil {
		n.LoanReference = *loanRow.LoanReference
	}
	if err := a.loanNotifier.NotifyRepaymentFailed(ctx, n); err != nil {
		a.logger.Error("failed to send the repayment failure notice",
			pkgErrors.AttrLoanID, loanID, "error", err)
	}
}

// FXBufferPct reports the buffer applied on the provider-Quoter path, after
// defaulting. Zero means entry rates are persisted unbuffered.
func (a *LoanServiceAdapter) FXBufferPct() float64 { return a.fxBuffer.Pct() }

// FXConfig tunes the entry-rate quoting that happens just before the loan is
// initiated against a provider. BufferPct is applied multiplicatively to the
// quoted sell rate (entry_rate_used = sell_rate * (1 - BufferPct)) and recorded
// on the loan for downstream drift detection.
type FXConfig struct {
	// BufferPct is a fraction (0.02 = 2 %). Nil defaults to
	// DefaultFXBufferPct; an explicit 0 persists the quoted rate unbuffered.
	BufferPct *float64
}

// lendingErr starts an error builder for loan origination and disbursement.
func lendingErr(op string) oops.OopsErrorBuilder {
	return oops.In(pkgErrors.DomainLending).Tags("loan").With(pkgErrors.AttrOperation, op)
}

// quoteErr starts an error builder for repayment quoting. The quote hard-fails
// rather than serving a stale figure, so every error here means the borrower
// was shown nothing rather than something wrong.
func quoteErr(loanID string) oops.OopsErrorBuilder {
	return oops.In(pkgErrors.DomainLending).
		Tags("repayment", "quote").
		With(pkgErrors.AttrOperation, "repayment_quote").
		With(pkgErrors.AttrLoanID, loanID)
}

// LoanAdapterDeps are the collaborators and settings the loan adapter needs.
// Optional members live here rather than in Set* so no half-built adapter exists.
type LoanAdapterDeps struct {
	LoanSvc      loan.Service
	ProductSvc   loanproduct.Service
	StellarSvc   stellar.Service
	OffRamps     *offramp.Registry
	LoanNotifier contracts.LoanNotifier
	TxnSvc       transaction.Service
	FXConfig     FXConfig
	Logger       *slog.Logger

	// Optional.
	FXOrchestrator  *moneygram.FXOrchestrator
	PublicBaseURL   string
	Shortener       urlshortener.Shortener
	AccountEnsurer  AccountEnsurer
	RepaymentAnchor *stellaranchor.Client
	// TreasuryAddress is where borrower cash deposits are credited. Required
	// alongside RepaymentAnchor; either alone leaves repayment unavailable.
	TreasuryAddress string
	// AnchorAuthAddress signs SEP-10 and seeds the child memo. Separate from
	// TreasuryAddress: where money lands vs which memo namespace it belongs to.
	AnchorAuthAddress string
	RepaymentWindow   time.Duration

	// RelayRouter routes a mobile-money disbursement to whichever provider
	// prices it best. Nil, or a router with routing off, leaves dispatch to
	// the off-ramp registry's own aliases.
	RelayRouter *relay.Router

	// CashIn resolves payment prompts. Nil, or a registry with no prompt
	// alias, disables PromptRepayment.
	CashIn *cashin.Registry
}

func NewLoanServiceAdapter(ctx context.Context, deps LoanAdapterDeps) (*LoanServiceAdapter, error) {
	loanSvc, productSvc, stellarSvc := deps.LoanSvc, deps.ProductSvc, deps.StellarSvc
	offRamps, loanNotifier, txnSvc := deps.OffRamps, deps.LoanNotifier, deps.TxnSvc
	fxCfg, logger := deps.FXConfig, deps.Logger

	if logger == nil {
		logger = slog.Default()
	}
	if offRamps == nil {
		return nil, lendingErr("new").With(pkgErrors.AttrDependency, "offramp_registry").
			Code(pkgErrors.CodeMissingDependency).Errorf("required dependency is missing")
	}
	// Load the highest-priority active product (ordered by priority_order ASC).
	products, err := productSvc.GetActive(ctx, services.Pagination{Page: 1, PageSize: 1})
	if err != nil {
		return nil, lendingErr("new").Code(pkgErrors.CodeLoanLoadFailed).
			Wrapf(err, "could not load the active loan product")
	}
	if len(products.Data) == 0 {
		return nil, lendingErr("new").Code(pkgErrors.CodeNotFound).
			Hint("run the 000006 migration to seed a loan product").
			Errorf("no active loan product is configured")
	}
	p := products.Data[0]

	schedule := "lump_sum"
	if len(p.AllowedRepaymentSchedules) > 0 {
		schedule = p.AllowedRepaymentSchedules[0]
	}

	originationFeeBps := int32(0)
	if p.OriginationFeeBps != nil {
		originationFeeBps = *p.OriginationFeeBps
	}
	cfg := &ussd.LoanProductConfig{
		ProductID:         p.ID,
		MinAmountCents:    p.MinAmount,
		MaxAmountCents:    p.MaxAmount,
		Currency:          p.Currency,
		DurationDays:      p.MinDurationDays,
		RepaymentSchedule: schedule,
		InterestRateBps:   p.InterestRateBps,
		OriginationFeeBps: originationFeeBps,
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
		relayRouter:   deps.RelayRouter,
		loanNotifier:  loanNotifier,
		txnSvc:        txnSvc,
		logger:        logger,
		productConfig: cfg,
		fxBuffer:      offramp.NewRateBuffer(fxCfg.BufferPct, DefaultFXBufferPct),
		dedupe:        newDedupeGate(60 * time.Second),

		fxOrch:              deps.FXOrchestrator,
		publicBaseURL:       deps.PublicBaseURL,
		shortener:           deps.Shortener,
		accountEnsurer:      deps.AccountEnsurer,
		repayAnchor:         deps.RepaymentAnchor,
		repayTreasuryPubkey: deps.TreasuryAddress,
		repayAuthPubkey:     deps.AnchorAuthAddress,
		repayWindow:         deps.RepaymentWindow,
		cashIn:              deps.CashIn,
	}, nil
}

// GetProductConfig returns the cached loan product configuration.
// Returns nil if no product was loaded (should not happen after successful construction).
func (a *LoanServiceAdapter) GetProductConfig() *ussd.LoanProductConfig {
	return a.productConfig
}

// RequestLoan implements ussd.LoanService. It orchestrates the full loan
// disbursement cycle: eligibility to create to approve to vault borrow to disburse
// to off-ramp to notify.
func (a *LoanServiceAdapter) RequestLoan(ctx context.Context, req *ussd.LoanRequest) (interface{}, error) {
	start := time.Now()

	// Amount validation is handled by the USSD handler against the loan product
	// config (fiat-denominated limits). By this point the request is pre-approved.

	// Round to whole USDC cents before Create, BorrowFromVault and Initiate:
	// anchors expect 2 decimals and a 7-decimal amount leaves them stuck.
	if rounded := roundToCentStroops(req.PrincipalAmount); rounded != req.PrincipalAmount {
		a.logger.Info("principal rounded to whole cents",
			"user_id", req.UserID,
			"original_stroops", req.PrincipalAmount,
			"rounded_stroops", rounded,
		)
		req.PrincipalAmount = rounded
	}

	payoutMethod := req.PayoutMethod
	if payoutMethod == "" {
		payoutMethod = offramp.PayoutMethodMobileMoney
	}

	// Cash-pickup needs a recipient name for SEP-9 prefill — fail before any
	// vault mutation rather than after MoneyGram rejects the withdraw call.
	if payoutMethod == offramp.PayoutMethodCashPickup && req.RecipientName == "" {
		a.logger.Error("cash-pickup loan rejected: recipient name missing",
			"user_id", req.UserID,
		)
		return nil, lendingErr("request_loan").Code(pkgErrors.CodeMissingAccount).
			Errorf("cash pickup needs a recipient name and the user has none on file")
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
	entryRate, entryRateSource, entryBufferPct := a.requoteEntryRate(ctx, providerOpts, req.LocalCurrency, req.CountryCode)

	// Step 1: Query dynamic vault APR; fall back to product rate.
	interestRateBps := a.productConfig.InterestRateBps
	aprWad, err := a.stellarSvc.GetBorrowAPR(ctx)
	if err != nil {
		a.logger.Warn("failed to fetch vault APR, using fallback", "error", err)
	} else if aprWad > 0 {
		interestRateBps = int32(aprWad / 1e14) // WAD (1e18) to bps (1e4)
	}

	// Step 2: Create loan record.
	productID := req.ProductID
	createResp, err := a.loanSvc.Create(ctx, loan.CreateLoanRequest{
		UserID:            req.UserID,
		AccountID:         req.AccountID,
		ProductID:         &productID,
		PrincipalAmount:   req.PrincipalAmount,
		PrincipalAsset:    req.PrincipalAsset,
		VaultAPRBps:       interestRateBps,
		OriginationFeeBps: a.productConfig.OriginationFeeBps,
		DurationDays:      req.DurationDays,
		RepaymentSchedule: req.RepaymentSched,
	})
	if err != nil {
		a.logger.Error("failed to create loan record", "user_id", req.UserID, "error", err)
		return nil, lendingErr("request_loan").Code(pkgErrors.CodeStateWriteFailed).
			Wrapf(err, "could not create the loan")
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
	a.persistEntryRate(ctx, loanID, entryRate, entryRateSource, entryBufferPct, req.LocalAmount, req.ChildAccountIndex)

	// Step 3: Auto-approve.
	// Use the nil UUID for system auto-approvals (approved_by is UUID in the DB).
	_, err = a.loanSvc.Approve(ctx, loanID, loan.ApproveLoanRequest{ApprovedBy: "00000000-0000-0000-0000-000000000000"})
	if err != nil {
		a.logger.Error("failed to approve loan", "loan_id", loanID, "error", err)
		return nil, lendingErr("request_loan").Code(pkgErrors.CodeStateWriteFailed).
			Wrapf(err, "could not approve the loan")
	}
	a.logger.Info("loan auto-approved", "loan_id", loanID)

	// Notify user of approval (best-effort) — use KES amount when available.
	if a.loanNotifier != nil && req.PhoneNumber != "" {
		loanRef := ""
		if createResp.LoanReference != nil {
			loanRef = *createResp.LoanReference
		}
		notification := contracts.LoanNotification{
			LoanID:          loanID,
			LoanReference:   loanRef,
			PhoneNumber:     req.PhoneNumber,
			DisplayAmount:   notifyAmount,
			DisplayCurrency: notifyCurrency,
		}
		// Cash-pickup loans get a scoped copy: the generic "Approved" template
		// implies a push disbursement, which is misleading here — a follow-up
		// SMS with the MoneyGram interactive URL is sent once the off-ramp
		// initiates (see recordSuccessfulInitiate).
		a.notifyAsync("approval", loanID, func(ctx context.Context) error {
			if payoutMethod == offramp.PayoutMethodCashPickup {
				return a.loanNotifier.NotifyLoanCashPickupApproved(ctx, notification)
			}
			return a.loanNotifier.NotifyLoanApproved(ctx, notification)
		})
	}

	// Ensure the borrower's child account exists on-chain before lending — it
	// is the fund-less identity we use for tracking/auditing, so we cannot
	// disburse without it. Normally created asynchronously at registration;
	// this is the safety net for that rare failure.
	if a.accountEnsurer != nil {
		if err := a.accountEnsurer.EnsureOnChainAccount(ctx, int(req.ChildAccountIndex), req.StellarAddress); err != nil {
			a.logger.Error("on-chain account ensure failed; aborting disbursement",
				"loan_id", loanID, "address", req.StellarAddress, "error", err)
			cancelStatus := "cancelled"
			_, _ = a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{VaultTxStatus: &cancelStatus})
			return nil, lendingErr("request_loan").Code(pkgErrors.CodeSubmitFailed).
				Wrapf(err, "could not ensure the borrower has an on-chain account")
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
		return nil, lendingErr("request_loan").Code(pkgErrors.CodeVaultRepayFailed).
			Wrapf(err, "vault borrow failed")
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
			UserID:           &req.UserID,
			AccountID:        &req.AccountID,
			LoanID:           &loanID,
			TxType:           models.TxTypeVaultBorrow,
			Amount:           borrowResp.AmountBorrowed,
			Asset:            "USDC",
			StellarTxHash:    &borrowResp.TxHash,
			StellarLedger:    &borrowResp.Ledger,
			ContractID:       &borrowResp.ContractID,
			ContractFunction: &borrowResp.ContractFunction,
			Description:      &vaultDesc,
		})
		if txnErr != nil {
			a.logger.Warn("failed to record vault borrow transaction", "loan_id", loanID, "error", txnErr)
		} else if txnResp != nil {
			// Transition: pending to submitted to success (vault TX is already confirmed).
			submittedStatus := models.TxStatusSubmitted
			_, _ = a.txnSvc.Update(ctx, txnResp.ID, transaction.UpdateTransactionRequest{
				Status: &submittedStatus,
			})
			successStatus := models.TxStatusSuccess
			_, _ = a.txnSvc.Update(ctx, txnResp.ID, transaction.UpdateTransactionRequest{
				Status:        &successStatus,
				StellarLedger: &borrowResp.Ledger,
			})
		}
	}

	// Settlement method and disbursement status are written by
	// recordSuccessfulInitiate; pre-stamping here raced the YC webhook.
	vaultTxStatus := "success"
	_, err = a.loanSvc.Disburse(ctx, loanID, loan.DisburseLoanRequest{
		VaultTxHash:   &borrowResp.TxHash,
		VaultTxStatus: &vaultTxStatus,
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
	if routed := a.routeMobileMoney(ctx, offrampReq, amountUSD); routed != "" {
		offrampReq.PayoutMethod = routed
	}
	provider, resolveErr := a.offRamps.Resolve(offrampReq)
	if resolveErr != nil {
		a.logger.Error("off-ramp registry resolve failed",
			"loan_id", loanID,
			"payout_method", payoutMethod,
			"error", resolveErr,
		)
		return nil, lendingErr("request_loan").Code(pkgErrors.CodeNotFound).
			Wrapf(resolveErr, "could not resolve an off-ramp provider")
	}
	offRampResult, err := provider.Initiate(ctx, offrampReq)
	offRampFailed := err != nil
	if offRampFailed {
		a.logger.Error("off-ramp failed — vault borrow succeeded, USDC in treasury, fiat not disbursed",
			"loan_id", loanID,
			"vault_tx_hash", borrowResp.TxHash,
			"error", err,
		)
		// Flip the loan's status so it is no longer treated as a live
		// disbursement. Distinct from LoanStatusDefaulted — the borrower owes
		// nothing. The disbursement view derives from this.
		if _, mErr := a.loanSvc.MarkAsOffRampFailed(ctx, loanID); mErr != nil {
			a.logger.Warn("failed to flip loan status to offramp_failed",
				"loan_id", loanID, "error", mErr)
		}

		// USDC never left treasury — repay vault immediately.
		a.repayVaultAfterInitiate(ctx, loanID, req.UserID, borrowResp.AmountBorrowed, "offramp_init_failed")

		// Notify user of failure (best-effort).
		if a.loanNotifier != nil && req.PhoneNumber != "" {
			loanRef := ""
			if createResp.LoanReference != nil {
				loanRef = *createResp.LoanReference
			}
			// Off-ramp failure SMS — distinct from the credit-default copy;
			// the borrower's USDC never left the treasury (or has been repaid).
			failedNote := contracts.LoanNotification{
				LoanID:          loanID,
				LoanReference:   loanRef,
				PhoneNumber:     req.PhoneNumber,
				DisplayAmount:   notifyAmount,
				DisplayCurrency: notifyCurrency,
			}
			a.notifyAsync("offramp_failed", loanID, func(ctx context.Context) error {
				return a.loanNotifier.NotifyLoanOffRampFailed(ctx, failedNote)
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

		// A fiat result means the YC adapter pivoted internally: the USDC is
		// still in treasury, so repay now rather than await the racy webhook.
		if payoutMethod == offramp.PayoutMethodMobileMoney &&
			offRampResult.SettlementMethod == string(yellowcard.SettlementMethodFiat) {
			a.repayVaultAfterInitiate(ctx, loanID, req.UserID, borrowResp.AmountBorrowed, "direct_to_fiat_pivot")
		}
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
	status := "disbursed"
	switch {
	case offRampFailed:
		status = "offramp_failed"
	case payoutMethod == offramp.PayoutMethodCashPickup:
		status = "awaiting_user"
	}
	return map[string]interface{}{
		"id":             loanID,
		"loan_reference": createResp.LoanReference,
		"status":         status,
		"total_amount":   totalAmount,
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
		// BirthDate is optional SEP-9 prefill — when absent the user supplies
		// it in MoneyGram's webview.
		return moneygram.Options{
			FirstName:          req.FirstName,
			LastName:           req.LastName,
			MobileNumber:       req.PhoneNumber,
			BirthDate:          req.BirthDate,
			Address:            req.Address,
			PostalCode:         req.PostalCode,
			City:               req.City,
			AddressCountryCode: req.AddressCountryCode,
			ChildAccountIndex:  req.ChildAccountIndex,
		}, nil
	default:
		return nil, lendingErr("request_loan").With("payout_method", string(payoutMethod)).
			Code(pkgErrors.CodeNotFound).Errorf("payout method is not supported")
	}
}

// requoteEntryRate returns (rate, source, bufferPct) with the safety buffer
// applied. A zero rate means no usable quote; callers skip persistence.
func (a *LoanServiceAdapter) requoteEntryRate(
	ctx context.Context,
	opts offramp.ProviderOptions,
	currency, countryCode string,
) (float64, string, float64) {
	if currency == "" {
		return 0, "", 0
	}

	if a.fxOrch != nil {
		res, err := a.fxOrch.Quote(ctx, moneygram.FXQuoteRequest{
			OriginatingCountry: moneygram.DefaultOriginatingCountry,
			DestinationCountry: stellaranchor.CountryISO3(countryCode),
			SendCurrency:       moneygram.DefaultSendCurrency,
			ReceiveCurrency:    currency,
		})
		if err == nil && res != nil && res.Rate > 0 {
			return res.Rate, res.Source, res.BufferPct
		}
		a.logger.Warn("FX orchestrator quote failed, falling back to provider Quoter",
			"currency", currency, "error", err)
	}

	p, err := a.offRamps.Resolve(offramp.Request{Options: opts})
	if err != nil {
		a.logger.Warn("entry-rate re-quote: registry resolve failed", "error", err)
		return 0, "", 0
	}
	quoter, ok := p.(offramp.Quoter)
	if !ok {
		return 0, "", 0
	}
	q, err := quoter.Quote(ctx, offramp.QuoteRequest{Currency: currency})
	if err != nil {
		a.logger.Warn("entry-rate re-quote failed",
			"provider", p.ID(), "currency", currency, "error", err)
		return 0, "", 0
	}
	// Sell only. Falling back to the buy rate would quote the borrower at the
	// wrong side of the spread, and nothing on the loan afterwards records
	// which side was used — entry_rate_source names the provider, not the leg.
	// No usable sell rate means no entry rate; the caller proceeds without one.
	if q.SellRate <= 0 {
		a.logger.Warn("entry-rate re-quote: provider returned no sell rate",
			"provider", p.ID(), "currency", currency)
		return 0, "", 0
	}
	return a.fxBuffer.Apply(q.SellRate), string(p.ID()), a.fxBuffer.Pct()
}

// persistEntryRate writes the entry-rate audit fields onto a fresh loan.
// Errors are logged, not returned: audit data, not load-bearing.
func (a *LoanServiceAdapter) persistEntryRate(
	ctx context.Context,
	loanID string,
	entryRate float64,
	entryRateSource string,
	entryBufferPct float64,
	localAmountCents int64,
	childAccountIndex uint32,
) {
	req := loan.UpdateLoanRequest{}
	any := false
	if e8 := creditmodels.RateE8(entryRate); e8 != nil {
		req.EntryRateBuffered = e8
		any = true
	}
	if entryRateSource != "" {
		v := entryRateSource
		req.EntryRateSource = &v
		any = true
	}
	if bps := creditmodels.BufferBps(entryBufferPct); bps != nil {
		req.EntryBufferBps = bps
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

// repayVaultAfterInitiate repays the borrowed USDC to the vault and records
// the on-chain hash + audit transaction. Used in two cases where the
// borrowed USDC is still in (or returned to) the treasury at Initiate time:
//   - off-ramp init failed entirely (USDC never moved)
//   - mobile-money direct→fiat pivot inside the YC adapter (direct push to
//     YC's wallet failed, so USDC is still in treasury; YC will front fiat)
//
// Idempotent via the existing VaultRepayTxHash column — a later webhook-
// triggered repay (DisbursementComplete + settlement_method=fiat) will see
// the hash and no-op.
func (a *LoanServiceAdapter) repayVaultAfterInitiate(
	ctx context.Context,
	loanID, userID string,
	amountStroops int64,
	trigger string,
) {
	repayResp, repayErr := a.stellarSvc.RepayToVault(ctx, stellar.RepayRequest{Amount: amountStroops})
	if repayErr != nil {
		a.logger.Error("CRITICAL: vault repay failed",
			"loan_id", loanID,
			"trigger", trigger,
			"amount_stroops", amountStroops,
			"error", repayErr,
		)
		failedStatus := "failed"
		_, _ = a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{VaultRepayStatus: &failedStatus})
		return
	}

	successStatus := "success"
	_, _ = a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		VaultRepayTxHash: &repayResp.TxHash,
		VaultRepayStatus: &successStatus,
	})

	if a.txnSvc != nil {
		desc := "Vault repay"
		txnResp, txnErr := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
			UserID:           &userID,
			LoanID:           &loanID,
			TxType:           models.TxTypeVaultRepay,
			Amount:           repayResp.AmountRepaid,
			Asset:            "USDC",
			StellarTxHash:    &repayResp.TxHash,
			StellarLedger:    &repayResp.Ledger,
			ContractID:       &repayResp.ContractID,
			ContractFunction: &repayResp.ContractFunction,
			Description:      &desc,
			Metadata:         txMetadata(map[string]any{"trigger": trigger}),
		})
		if txnErr == nil && txnResp != nil {
			submittedStatus := models.TxStatusSubmitted
			_, _ = a.txnSvc.Update(ctx, txnResp.ID, transaction.UpdateTransactionRequest{
				Status: &submittedStatus,
			})
			successStatus := models.TxStatusSuccess
			_, _ = a.txnSvc.Update(ctx, txnResp.ID, transaction.UpdateTransactionRequest{
				Status:        &successStatus,
				StellarLedger: &repayResp.Ledger,
			})
		}
	}

	a.logger.Info("vault repaid",
		"loan_id", loanID,
		"trigger", trigger,
		"repay_tx_hash", repayResp.TxHash,
		"amount_repaid", repayResp.AmountRepaid,
	)
}

// shortCodeTTL bounds the life of a /r/{code} interactive redirect. MoneyGram
// expires the SEP-24 session itself; this only stops an unauthenticated code
// staying resolvable forever on a loan that never reaches a terminal state.
const shortCodeTTL = 24 * time.Hour

// mintRedirectLink generates a /r/{code} redirect and persists it. Returns
// rawURL unchanged on failure: a long link resolves, a 404 does not.
func (a *LoanServiceAdapter) mintRedirectLink(ctx context.Context, loanID, rawURL string) string {
	code, err := newShortCode()
	if err != nil {
		a.logger.Warn("short-code generation failed; sending raw URL",
			"loan_id", loanID, "error", err)
		return rawURL
	}

	expiresAt := time.Now().Add(shortCodeTTL)
	if _, err := a.loanSvc.Update(ctx, loanID, loan.UpdateLoanRequest{
		RampShortCode:          &code,
		RampShortCodeExpiresAt: &expiresAt,
	}); err != nil {
		a.logger.Warn("failed to persist short code; sending raw URL",
			"loan_id", loanID, "error", err)
		return rawURL
	}
	return a.publicBaseURL + "/r/" + code
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
		var rawURL string
		if mg, ok := result.Provider.(moneygram.CashPickupPayload); ok && mg.InteractiveURL != "" {
			rawURL = mg.InteractiveURL
			updateReq.RampInteractiveURL = &rawURL
		}

		initiated := true
		if _, err := a.loanSvc.Update(ctx, loanID, updateReq); err != nil {
			a.logger.Warn("failed to record cash-pickup initiation",
				"loan_id", loanID, "error", err)
			initiated = false
		}

		// Shorten via dub (branded short domain + rich preview), pointed at the
		// MoneyGram URL rather than at /r/{code}.
		smsLink, shortenErr := shortenedLink(ctx, a.shortener, rawURL, "")
		if shortenErr != nil {
			a.logger.Warn("dub shorten failed; falling back to the internal redirect",
				"loan_id", loanID, "error", shortenErr)
		}

		// A /r/{code} code is minted only when dub produced nothing. It is an
		// unauthenticated bearer token resolving to a live KYC session, so one
		// per pickup — not one alongside every dub link.
		if smsLink == "" && rawURL != "" {
			smsLink = rawURL
			if initiated && a.publicBaseURL != "" {
				smsLink = a.mintRedirectLink(ctx, loanID, rawURL)
			}
		}

		// SMS user the interactive URL so they can complete KYC.
		if a.loanNotifier != nil && req.PhoneNumber != "" && smsLink != "" {
			notification := contracts.LoanNotification{
				LoanID:          loanID,
				PhoneNumber:     req.PhoneNumber,
				DisplayAmount:   float64(req.PrincipalAmount) / 1e7,
				DisplayCurrency: "USD",
				InteractiveURL:  smsLink,
			}
			if createResp.LoanReference != nil {
				notification.LoanReference = *createResp.LoanReference
			}
			a.notifyAsync("cash_pickup_initiated", loanID, func(ctx context.Context) error {
				return a.loanNotifier.NotifyLoanCashPickupInitiated(ctx, notification)
			})
		}

	default:
		// Mobile-money: existing YC path — locked AmountLocal + fees.
		rampFiatAmount := int64(result.AmountLocal * 100) // cents
		updateReq.RampFiatAmount = &rampFiatAmount
		updateReq.RampFiatCurr = &result.LocalCurrency

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
		if e8 := creditmodels.RateE8(req.ConversionRate); e8 != nil {
			updateReq.DisbursementRate = e8
		}

		if _, err := a.loanSvc.Update(ctx, loanID, updateReq); err != nil {
			a.logger.Warn("failed to record off-ramp details", "loan_id", loanID, "error", err)
		} else {
			a.logger.Info("loan updated with off-ramp details", "loan_id", loanID)
		}

		// Record off-ramp transaction.
		if a.txnSvc != nil {
			offRampDesc := fmt.Sprintf("Off-ramp via %s", providerID)
			provName := string(providerID)
			if _, txnErr := a.txnSvc.Create(ctx, transaction.CreateTransactionRequest{
				UserID:           &req.UserID,
				AccountID:        &req.AccountID,
				LoanID:           &loanID,
				TxType:           models.TxTypeOffRamp,
				Amount:           rampFiatAmount,
				Asset:            result.LocalCurrency,
				ExternalID:       &result.RequestID,
				ExternalProvider: &provName,
				Description:      &offRampDesc,
				Metadata: txMetadata(map[string]any{
					"settlement_method": result.SettlementMethod,
					"sequence_id":       result.SequenceID,
				}),
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
			"loan_reference":         l.LoanReference,
			"status":                 l.Status,
			"repayment_status":       l.RepaymentStatus,
			"due_date":               l.DueDate,
			"delivered_amount_local": l.DeliveredAmountLocal,
			"borrow_index":           l.BorrowIndex,
			"service_fee_usd":        l.ServiceFeeUSD,
			"service_fee_local":      l.ServiceFeeLocal,
		}
	}
	a.logger.Info("fetched user loans", "user_id", userID, "count", len(results))
	return results, nil
}

// CheckLoanEligibility implements ussd.LoanService. Fiat limits are checked by
// the USSD handler; this fetches the vault APR and approves.
func (a *LoanServiceAdapter) CheckLoanEligibility(ctx context.Context, userID string, amount int64, duration int) (*ussd.LoanApproval, error) {
	// Fetch dynamic APR from vault; fall back to product rate.
	fallbackRate := float64(a.productConfig.InterestRateBps) / 10000.0 // bps to decimal
	interestRate := fallbackRate
	aprWad, err := a.stellarSvc.GetBorrowAPR(ctx)
	if err != nil {
		a.logger.Warn("failed to fetch vault APR for eligibility, using fallback", "error", err)
	} else if aprWad > 0 {
		interestRate = float64(aprWad) / 1e18 // WAD to decimal (e.g. 0.08 for 8%)
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
		InterestRate: interestRate * 100, // decimal to percentage (e.g. 8.0 for 8%)
	}, nil
}

// GetRepaymentQuote recomputes the amount owed from the vault's current
// borrow_index and the latest FX rate. Hard-fails rather than serve a stale
// number the borrower might act on.
//
// Math:
//
//	amount_usdc  = principal * (current_borrow_index / origination_borrow_index)
//	amount_local = amount_usdc * fx_rate
//
// Rounded up to the nearest stroop/cent so the borrower never underpays.
func (a *LoanServiceAdapter) GetRepaymentQuote(ctx context.Context, loanID string) (*ussd.RepaymentQuote, error) {
	resp, err := a.loanSvc.GetByID(ctx, loanID)
	if err != nil {
		return nil, quoteErr(loanID).Code(pkgErrors.CodeLoanLoadFailed).
			Wrapf(err, "could not load the loan")
	}
	if resp.BorrowIndex == nil || *resp.BorrowIndex <= 0 {
		return nil, quoteErr(loanID).Code(pkgErrors.CodeIncompleteResponse).
			Errorf("loan has no origination borrow index to quote against")
	}
	originIndex := *resp.BorrowIndex

	currentIndex, err := a.stellarSvc.GetBorrowIndex(ctx)
	if err != nil {
		return nil, quoteErr(loanID).Code(pkgErrors.CodeQuoteFailed).
			Wrapf(err, "could not read the vault borrow index")
	}
	if currentIndex < originIndex {
		// Index only grows; this would mean we read a stale or wrong value.
		return nil, quoteErr(loanID).
			With("current_index", currentIndex).
			With("origination_index", originIndex).
			Code(pkgErrors.CodeQuoteFailed).
			Errorf("vault borrow index went backwards, which means a stale or wrong read")
	}

	// principal * current / origin, rounded up — favors the protocol.
	amountUSDC := mulDivCeil(resp.PrincipalAmount, currentIndex, originIndex)

	if resp.ServiceFeeUSD != nil && *resp.ServiceFeeUSD > 0 {
		amountUSDC += *resp.ServiceFeeUSD * 1e5
	}

	currency := "KES"
	if resp.RampFiatCurr != nil && *resp.RampFiatCurr != "" {
		currency = *resp.RampFiatCurr
	}

	fxRate, fxSource, fxErr := a.fetchFXForQuote(ctx, currency)
	if fxErr != nil {
		return nil, quoteErr(loanID).With(pkgErrors.AttrCurrency, currency).
			Code(pkgErrors.CodeRateUnavailable).Wrapf(fxErr, "could not fetch an FX rate")
	}

	amountLocalCents := int64(0)
	if fxRate > 0 {
		// stroops → USD → local → cents, ceil.
		amountUSD := float64(amountUSDC) / 1e7
		amountLocalCents = int64(amountUSD*fxRate*100 + 0.999)
	}

	return &ussd.RepaymentQuote{
		LoanID:             loanID,
		AmountUSDCStroops:  amountUSDC,
		AmountLocalCents:   amountLocalCents,
		LocalCurrency:      currency,
		BorrowIndexAtQuote: currentIndex,
		FXRate:             fxRate,
		QuoteSource:        fxSource,
		AsOf:               time.Now(),
	}, nil
}

// PromptRepayment implements ussd.RepaymentPrompter. It pushes a prompt at
// the borrower's handset for the full payoff, converted to whole shillings
// rounded up — M-Pesa accepts whole KES only, and a shilling short is a loan
// that never settles.
func (a *LoanServiceAdapter) PromptRepayment(ctx context.Context, loanID, phoneNumber string) error {
	if a.cashIn == nil {
		return lendingErr("prompt_repayment").With(pkgErrors.AttrLoanID, loanID).
			Code(pkgErrors.CodeAnchorNotWired).Errorf("no cash-in provider is configured")
	}

	quote, err := a.GetRepaymentQuote(ctx, loanID)
	if err != nil {
		return err
	}
	if quote.LocalCurrency != "KES" {
		return lendingErr("prompt_repayment").With(pkgErrors.AttrLoanID, loanID).
			With(pkgErrors.AttrCurrency, quote.LocalCurrency).
			Code(pkgErrors.CodeUnsupportedOperation).Errorf("prompts push M-Pesa, which collects in KES")
	}
	amountKES := (quote.AmountLocalCents + 99) / 100

	provider, err := a.cashIn.Resolve(cashin.Request{
		LoanID:           loanID,
		CollectionMethod: cashin.CollectionMethodPrompt,
	})
	if err != nil {
		return lendingErr("prompt_repayment").With(pkgErrors.AttrLoanID, loanID).
			Wrapf(err, "could not resolve a prompt provider")
	}
	prompter, ok := provider.(cashin.Prompter)
	if !ok {
		return lendingErr("prompt_repayment").With(pkgErrors.AttrLoanID, loanID).
			Code(pkgErrors.CodeUnsupportedOperation).Errorf("the resolved provider cannot push prompts")
	}
	if _, err := prompter.Prompt(ctx, cashin.PromptRequest{
		LoanID:    loanID,
		Payer:     phoneNumber,
		AmountKES: amountKES,
	}); err != nil {
		return lendingErr("prompt_repayment").With(pkgErrors.AttrLoanID, loanID).
			With(pkgErrors.AttrAmountLocal, amountKES).
			Wrapf(err, "the prompt was refused")
	}
	return nil
}

// fetchFXForQuote sources a current FX rate using the orchestrator cascade
// (MG primary → YC fallback → stale cache). Returns rate + source label.
func (a *LoanServiceAdapter) fetchFXForQuote(ctx context.Context, currency string) (float64, string, error) {
	if a.fxOrch != nil {
		res, err := a.fxOrch.Quote(ctx, moneygram.FXQuoteRequest{
			OriginatingCountry: moneygram.DefaultOriginatingCountry,
			DestinationCountry: stellaranchor.CountryISO3("KE"),
			SendCurrency:       moneygram.DefaultSendCurrency,
			ReceiveCurrency:    currency,
		})
		if err == nil && res != nil && res.Rate > 0 {
			return res.Rate, res.Source, nil
		}
		return 0, "", lendingErr("quote_fx").Code(pkgErrors.CodeRateUnavailable).
			Wrapf(err, "FX orchestrator produced no rate")
	}
	// No orchestrator wired — try the YC adapter's Quoter directly.
	provider, err := a.offRamps.Resolve(offramp.Request{
		PayoutMethod: offramp.PayoutMethodMobileMoney,
		Options:      yellowcard.Options{SettlementMethod: yellowcard.SettlementMethodDirect},
	})
	if err != nil {
		return 0, "", lendingErr("quote_fx").Code(pkgErrors.CodeNotFound).
			Wrapf(err, "could not resolve an FX provider")
	}
	quoter, ok := provider.(offramp.Quoter)
	if !ok {
		return 0, "", lendingErr("quote_fx").With(pkgErrors.AttrProvider, string(provider.ID())).
			Code(pkgErrors.CodeRateUnavailable).Errorf("provider exposes no quoter")
	}
	q, err := quoter.Quote(ctx, offramp.QuoteRequest{Currency: currency})
	if err != nil {
		return 0, "", err
	}
	// Sell only — see requoteEntryRate. A repayment quoted at the buy rate is
	// wrong by the spread, and silently so.
	if q.SellRate <= 0 {
		return 0, "", lendingErr("quote_fx").With(pkgErrors.AttrCurrency, currency).
			Code(pkgErrors.CodeRateUnavailable).Errorf("quoter returned no sell rate")
	}
	return q.SellRate, string(provider.ID()), nil
}

// mulDivCeil computes ceil(a * b / c) without overflow on the intermediate
// multiplication, using big.Int. Used for the (principal * current_index /
// origination_index) calc where intermediate can exceed int64.
func mulDivCeil(a, b, c int64) int64 {
	ai := big.NewInt(a)
	bi := big.NewInt(b)
	ci := big.NewInt(c)
	num := new(big.Int).Mul(ai, bi)
	q, r := new(big.Int).QuoRem(num, ci, new(big.Int))
	if r.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	return q.Int64()
}

// strDeref safely dereferences an optional string for log output.
func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
