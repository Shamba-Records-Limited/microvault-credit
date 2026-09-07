package adapters

import (
	"context"
	"time"

	"github.com/samber/lo"
	"github.com/samber/oops"

	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/cashin"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/mpesa"

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
}

// MpesaCollectionAdapterDeps are the collaborators the adapter needs; all
// required.
type MpesaCollectionAdapterDeps struct {
	Client  *mpesa.Client
	Repo    repository.LoanRepository
	LoanSvc loan.Service
	Config  config.MpesaConfig
}

// NewMpesaCollectionAdapter builds the adapter.
func NewMpesaCollectionAdapter(deps MpesaCollectionAdapterDeps) (*MpesaCollectionAdapter, error) {
	if deps.Client == nil || deps.Repo == nil || deps.LoanSvc == nil {
		return nil, oops.In(pkgErrors.DomainRepaymentCashIn).Tags("mpesa", "collection").
			Code(pkgErrors.CodeMissingDependency).Errorf("client, loan repository and loan service are required")
	}
	return &MpesaCollectionAdapter{
		client:  deps.Client,
		repo:    deps.Repo,
		loanSvc: deps.LoanSvc,
		cfg:     deps.Config,
		now:     time.Now,
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
		callbackURL = a.cfg.CallbackBaseURL + "/callbacks/daraja/" + a.cfg.CallbackSlug + "/stk/result"
	}

	resp, err := a.client.Express(ctx, mpesa.ExpressRequest{
		AmountKES:        req.AmountKES,
		Payer:            req.Payer,
		AccountReference: reference,
		CallbackURL:      callbackURL,
	})
	if err != nil {
		return nil, adapterErr("mpesa_prompt", req.LoanID).Wrapf(err, "express push failed")
	}
	if !resp.Accepted() {
		return nil, adapterErr("mpesa_prompt", req.LoanID).Wrapf(mpesa.ExpressRejection(resp), "express push was declined")
	}

	nextPoll := a.now().Add(a.cfg.STKPollInterval)
	if _, err := a.loanSvc.Update(ctx, req.LoanID, loan.UpdateLoanRequest{
		RepaymentProvider:        lo.ToPtr(models.LoanRepaymentProviderMpesa),
		RepaymentStatus:          lo.ToPtr(models.LoanRepaymentStatusInitiated),
		RepaymentMpesaCheckoutID: lo.ToPtr(resp.CheckoutRequestID),
		RepaymentNextPollAt:      &nextPoll,
	}); err != nil {
		return nil, adapterErr("mpesa_prompt", req.LoanID).Code(pkgErrors.CodeStateWriteFailed).Wrapf(err, "could not mark the repayment initiated")
	}

	return &cashin.PromptResult{
		LoanID:    req.LoanID,
		Reference: reference,
		Provider: mpesa.ExpressPayload{
			MerchantRequestID: resp.MerchantRequestID,
			CheckoutRequestID: resp.CheckoutRequestID,
			CustomerMessage:   resp.CustomerMessage,
			PromptedAmountKES: req.AmountKES,
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
