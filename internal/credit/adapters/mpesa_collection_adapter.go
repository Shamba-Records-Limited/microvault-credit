package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/samber/oops"

	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	coremodels "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/cashin"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/mpesa"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/user"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
)

// Compile-time checks.
var (
	_ cashin.Collector    = (*MpesaCollectionAdapter)(nil)
	_ cashin.Prompter     = (*MpesaCollectionAdapter)(nil)
	_ cashin.StatusReader = (*MpesaCollectionAdapter)(nil)
)

// userLookup is the one method Mobile Number Validation needs from
// user.Service — a narrow interface so this file depends on a capability, not
// the whole service.
type userLookup interface {
	GetByID(ctx context.Context, id string) (*user.UserResponse, error)
}

// MpesaCollectionAdapter opens loan collections on the M-Pesa rail. The
// paybill method is passive — instructions only, settled by C2B — and STK
// prompts go through Prompter, which needs the payer MSISDN a plain Request
// does not carry.
type MpesaCollectionAdapter struct {
	client  *mpesa.Client
	repo    repository.LoanRepository
	loanSvc loan.Service
	cfg     config.MpesaConfig
	now     func() time.Time

	// userSvc and validationRepo are optional: nil disables Mobile Number
	// Validation entirely, independent of the configured policy, so a
	// deployment that never wires them never spends on Daraja's per-call fee.
	userSvc          userLookup
	validationRepo   corerepository.MpesaNumberValidationRepository
	validationPolicy mpesa.ValidationPolicy
	logger           *slog.Logger
}

// MpesaCollectionAdapterDeps are the collaborators the adapter needs.
// UserSvc, ValidationRepo, ValidationPolicy and Logger are optional — leaving
// UserSvc or ValidationRepo nil disables Mobile Number Validation regardless
// of ValidationPolicy.
type MpesaCollectionAdapterDeps struct {
	Client  *mpesa.Client
	Repo    repository.LoanRepository
	LoanSvc loan.Service
	Config  config.MpesaConfig

	UserSvc          userLookup
	ValidationRepo   corerepository.MpesaNumberValidationRepository
	ValidationPolicy mpesa.ValidationPolicy
	Logger           *slog.Logger
}

// NewMpesaCollectionAdapter builds the adapter.
func NewMpesaCollectionAdapter(deps MpesaCollectionAdapterDeps) (*MpesaCollectionAdapter, error) {
	if deps.Client == nil || deps.Repo == nil || deps.LoanSvc == nil {
		return nil, oops.In(pkgErrors.DomainRepaymentCashIn).Tags("mpesa", "collection").
			Code(pkgErrors.CodeMissingDependency).Errorf("client, loan repository and loan service are required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &MpesaCollectionAdapter{
		client:           deps.Client,
		repo:             deps.Repo,
		loanSvc:          deps.LoanSvc,
		cfg:              deps.Config,
		now:              time.Now,
		userSvc:          deps.UserSvc,
		validationRepo:   deps.ValidationRepo,
		validationPolicy: deps.ValidationPolicy,
		logger:           logger.With("component", "mpesa_collection_adapter"),
	}, nil
}

// ID names the provider.
func (a *MpesaCollectionAdapter) ID() cashin.ProviderID { return cashin.ProviderMpesa }

// Collect opens the passive paybill collection: the loan's short reference is
// the account reference the borrower pays under, and settlement arrives via
// C2B confirmation and Pull rather than anything this call does.
func (a *MpesaCollectionAdapter) Collect(ctx context.Context, req cashin.Request) (*cashin.Result, error) {
	if req.CollectionMethod == cashin.CollectionMethodPrompt {
		return nil, adapterErr("mpesa_collect", req.LoanID).
			Code(pkgErrors.CodeUnsupportedOperation).
			Errorf("STK prompts go through Prompter; Collect opens the passive paybill rail")
	}

	l, err := a.repo.GetByID(ctx, req.LoanID)
	if err != nil {
		return nil, adapterErr("mpesa_collect", req.LoanID).Code(pkgErrors.CodeLoanLoadFailed).Wrapf(err, "could not load loan")
	}
	if l.LoanReference == nil || *l.LoanReference == "" {
		return nil, adapterErr("mpesa_collect", req.LoanID).Code(pkgErrors.CodeMissingDependency).Errorf("loan has no short reference")
	}

	return &cashin.Result{
		LoanID:    req.LoanID,
		Reference: *l.LoanReference,
		Amount:    req.AmountMinor,
		Provider: mpesa.PayBillPayload{
			Shortcode:        a.cfg.CollectionShortcode,
			AccountReference: *l.LoanReference,
		},
	}, nil
}

// Prompt pushes an STK prompt at the payer and marks the loan's repayment
// initiated, handing the checkout to the loan poller via
// repayment_mpesa_checkout_id and repayment_next_poll_at.
func (a *MpesaCollectionAdapter) Prompt(ctx context.Context, req cashin.PromptRequest) (*cashin.PromptResult, error) {
	l, err := a.repo.GetByID(ctx, req.LoanID)
	if err != nil {
		return nil, adapterErr("mpesa_prompt", req.LoanID).Code(pkgErrors.CodeLoanLoadFailed).Wrapf(err, "could not load loan")
	}
	if l.LoanReference == nil || *l.LoanReference == "" {
		return nil, adapterErr("mpesa_prompt", req.LoanID).Code(pkgErrors.CodeMissingDependency).Errorf("loan has no short reference")
	}
	if l.RepaymentStatus == models.LoanRepaymentStatusInitiated {
		return nil, adapterErr("mpesa_prompt", req.LoanID).Code(pkgErrors.CodeRepaymentInFlight).
			Errorf("a repayment prompt is already in flight for this loan")
	}
	reference := *l.LoanReference

	callbackURL := req.CallbackURL
	if callbackURL == "" {
		// The base URL is the bare host; the route lives under /api/v1, so the
		// prefix is added here rather than carried in the configured value.
		base := strings.TrimRight(a.cfg.CallbackBaseURL, "/")
		callbackURL = base + "/api/v1/callbacks/daraja/" + a.cfg.CallbackSlug + "/stk/result"
	}

	// Sandbox has no simulator: a handset must be charged a real amount, so a
	// configured override replaces the payoff with a small fixed figure.
	amountKES := req.AmountKES
	if a.cfg.PromptAmountKES > 0 {
		amountKES = int64(a.cfg.PromptAmountKES)
	}

	resp, err := a.client.Express(ctx, mpesa.ExpressRequest{
		AmountKES:        amountKES,
		Payer:            req.Payer,
		AccountReference: reference,
		CallbackURL:      callbackURL,
		// The loan reference doubles as the prompt's description: thirteen
		// characters is all Daraja allows, and the nine-char reference fits.
		TransactionDesc: reference,
	})
	if err != nil {
		return nil, adapterErr("mpesa_prompt", req.LoanID).Wrapf(err, "express push failed")
	}
	if !resp.Accepted() {
		return nil, adapterErr("mpesa_prompt", req.LoanID).Wrapf(mpesa.ExpressRejection(resp), "express push was declined")
	}

	// The payoff lock is req.AmountUSDCStroops, never derived from amountKES
	// — amountKES may be the sandbox override above, a fixed tiny figure with
	// no relationship to what the loan actually owes. mpesa-settle's on-chain
	// repay reads this column, so a sandbox run must still lock the real
	// payoff, not the amount charged on the test handset.
	nextPoll := a.now().Add(a.cfg.STKPollInterval)
	lockedAt := a.now()
	if _, err := a.loanSvc.Update(ctx, req.LoanID, loan.UpdateLoanRequest{
		RepaymentProvider:        lo.ToPtr(models.LoanRepaymentProviderMpesa),
		RepaymentStatus:          lo.ToPtr(models.LoanRepaymentStatusInitiated),
		RepaymentMpesaCheckoutID: lo.ToPtr(resp.CheckoutRequestID),
		RepaymentNextPollAt:      &nextPoll,
		RepaymentPayoffStroops:   lo.ToPtr(req.AmountUSDCStroops),
		RepaymentLockedAt:        &lockedAt,
	}); err != nil {
		return nil, adapterErr("mpesa_prompt", req.LoanID).Code(pkgErrors.CodeStateWriteFailed).Wrapf(err, "could not mark the repayment initiated")
	}

	// Out of band, never inside the request that pushed the prompt: Mobile
	// Number Validation is a paid, synchronous third-party call, and nothing
	// about whether the payer's MSISDN matches their national ID should slow
	// down or fail the STK push itself.
	if a.validationEnabled() {
		go a.validateNumber(l.UserID, req.Payer)
	}

	return &cashin.PromptResult{
		LoanID:    req.LoanID,
		Reference: reference,
		Provider: mpesa.ExpressPayload{
			MerchantRequestID: resp.MerchantRequestID,
			CheckoutRequestID: resp.CheckoutRequestID,
			CustomerMessage:   resp.CustomerMessage,
			PromptedAmountKES: amountKES,
		},
	}, nil
}

// Status resolves a checkout by express query. Amount is not reported by the
// query endpoint; the caller supplies it from the initiated request.
func (a *MpesaCollectionAdapter) Status(ctx context.Context, ref cashin.ProviderRef) (*cashin.Status, error) {
	resp, err := a.client.ExpressQuery(ctx, ref.ID, a.cfg.CollectionShortcode)
	if err != nil {
		return nil, adapterErr("mpesa_status", "").Wrapf(err, "express query failed")
	}
	code, _ := resp.Outcome()
	return &cashin.Status{
		Reference: ref.ID,
		Succeeded: code == 0,
		Provider: mpesa.ExpressPayload{
			MerchantRequestID: resp.MerchantRequestID,
			CheckoutRequestID: resp.CheckoutRequestID,
		},
	}, nil
}

// validationEnabled reports whether Mobile Number Validation should run.
// Disabled by an unset/"disabled" policy or by either collaborator being
// nil — the platform validating upstream and this adapter not being wired
// for it look the same from here, which is the point: neither should spend
// on Daraja's per-call fee.
func (a *MpesaCollectionAdapter) validationEnabled() bool {
	if a.userSvc == nil || a.validationRepo == nil {
		return false
	}
	switch a.validationPolicy {
	case mpesa.ValidationAdvisory, mpesa.ValidationEnforcing:
		return true
	default:
		return false
	}
}

// validateNumber checks the payer's MSISDN against the borrower's national ID
// and records the verdict. Best-effort throughout: every failure is logged
// and swallowed, because this is a fraud signal for risk scoring, not a
// condition the STK push itself depends on. Enforcing's blocking behaviour is
// not implemented here — see config.MpesaConfig.NumberValidationPolicy.
func (a *MpesaCollectionAdapter) validateNumber(userID, msisdn string) {
	// Its own context, not the request's: this runs after the STK push has
	// already responded, so a context tied to that request would be
	// cancelled before this starts. Bounded so a slow Daraja or a slow user
	// lookup cannot leak the goroutine forever.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u, err := a.userSvc.GetByID(ctx, userID)
	if err != nil || u.NationalID == "" {
		return
	}

	hash := identityHash(msisdn, mpesa.IDTypeNational, u.NationalID)
	if _, err := a.validationRepo.Get(ctx, hash); err == nil {
		// Already cached. No scheduled re-validation exists yet, so a cache
		// hit is treated as good until something adds one.
		return
	}

	result, err := a.client.ValidateMobileNumber(ctx, msisdn, mpesa.IDTypeNational, u.NationalID, a.cfg.CollectionShortcode)
	if err != nil {
		a.logger.Warn("mobile number validation call failed", "user_id", userID, "error", err)
		return
	}

	if err := a.validationRepo.Upsert(ctx, &coremodels.MpesaNumberValidation{
		IdentityHash: hash,
		Matched:      result.Matched,
		ResponseCode: result.ResponseCode,
		CheckedAt:    time.Now(),
	}); err != nil {
		a.logger.Warn("could not cache mobile number validation verdict", "user_id", userID, "error", err)
	}
}

// identityHash never persists the tuple itself, only its digest — the cache
// exists to avoid paying for the same check twice, not to store national ID
// numbers.
func identityHash(msisdn string, idType mpesa.IDType, idNumber string) string {
	sum := sha256.Sum256([]byte(msisdn + "|" + string(idType) + "|" + idNumber))
	return hex.EncodeToString(sum[:])
}
