package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"
	"github.com/samber/oops"

	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/airtel"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/cashin"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
)

// Compile-time checks.
var (
	_ cashin.Collector    = (*AirtelCollectionAdapter)(nil)
	_ cashin.Prompter     = (*AirtelCollectionAdapter)(nil)
	_ cashin.StatusReader = (*AirtelCollectionAdapter)(nil)
	_ cashin.Reverser     = (*AirtelCollectionAdapter)(nil)
)

// airtelErr is the error builder for this adapter.
func airtelErr(op, loanID string) oops.OopsErrorBuilder {
	return oops.
		In(pkgErrors.DomainRepaymentCashIn).
		Tags("airtel", "collection").
		With(pkgErrors.AttrProvider, "airtel").
		With(pkgErrors.AttrOperation, op).
		With(pkgErrors.AttrLoanID, loanID)
}

// AirtelCollectionAdapter opens loan collections on the Airtel Money rail.
//
// The rail is push-only. Airtel Collection has no passive equivalent of a
// paybill a borrower can walk up and pay, so every collection here is a USSD
// push and Collect refuses.
//
// The borrower reaches this adapter by choosing "Airtel Money" in the repay
// menu, which pins the provider through cashin.Request.Options. The registry's
// method aliases still point at M-Pesa: a carrier guessed from an MSISDN
// prefix goes stale silently as the regulator reallocates ranges, and a
// mis-routed prompt is a support call.
type AirtelCollectionAdapter struct {
	client  *airtel.Client
	repo    repository.LoanRepository
	loanSvc loan.Service
	cfg     config.AirtelConfig
	now     func() time.Time
	logger  *slog.Logger
}

// AirtelCollectionAdapterDeps are the collaborators the adapter needs. Logger
// is optional.
type AirtelCollectionAdapterDeps struct {
	Client  *airtel.Client
	Repo    repository.LoanRepository
	LoanSvc loan.Service
	Config  config.AirtelConfig

	Logger *slog.Logger
}

// NewAirtelCollectionAdapter builds the adapter.
func NewAirtelCollectionAdapter(deps AirtelCollectionAdapterDeps) (*AirtelCollectionAdapter, error) {
	if deps.Client == nil || deps.Repo == nil || deps.LoanSvc == nil {
		return nil, oops.In(pkgErrors.DomainRepaymentCashIn).Tags("airtel", "collection").
			Code(pkgErrors.CodeMissingDependency).
			Errorf("client, loan repository and loan service are all required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &AirtelCollectionAdapter{
		client:  deps.Client,
		repo:    deps.Repo,
		loanSvc: deps.LoanSvc,
		cfg:     deps.Config,
		now:     time.Now,
		logger:  logger.With("component", "airtel_collection_adapter"),
	}, nil
}

// ID names the provider.
func (a *AirtelCollectionAdapter) ID() cashin.ProviderID { return cashin.ProviderAirtel }

// Collect refuses.
//
// Collector is the registry's one mandatory capability, so the method has to
// exist. Airtel Collection is push-only, and returning a passive instruction
// the borrower cannot act on would be worse than saying so.
func (a *AirtelCollectionAdapter) Collect(_ context.Context, req cashin.Request) (*cashin.Result, error) {
	return nil, airtelErr("airtel_collect", req.LoanID).
		Code(pkgErrors.CodeUnsupportedOperation).
		Hint("Airtel Collection is a USSD push; use Prompter.").
		Errorf("the airtel rail has no passive collection")
}

// Prompt pushes a USSD payment request at the payer and marks the loan's
// repayment initiated, handing the transaction to the enquiry poller via
// repayment_airtel_txn_id and repayment_next_poll_at.
//
// Unlike the M-Pesa rail, no callback URL travels with the request: Airtel
// configures one callback path per application in its own portal, so
// req.CallbackURL is ignored here.
func (a *AirtelCollectionAdapter) Prompt(ctx context.Context, req cashin.PromptRequest) (*cashin.PromptResult, error) {
	l, err := a.repo.GetByID(ctx, req.LoanID)
	if err != nil {
		return nil, airtelErr("airtel_prompt", req.LoanID).Code(pkgErrors.CodeLoanLoadFailed).Wrapf(err, "could not load loan")
	}
	if l.LoanReference == nil || *l.LoanReference == "" {
		return nil, airtelErr("airtel_prompt", req.LoanID).Code(pkgErrors.CodeMissingDependency).Errorf("loan has no short reference")
	}
	if l.RepaymentStatus == models.LoanRepaymentStatusInitiated {
		return nil, airtelErr("airtel_prompt", req.LoanID).Code(pkgErrors.CodeRepaymentInFlight).
			Errorf("a repayment prompt is already in flight for this loan")
	}
	reference := *l.LoanReference

	// Staging has no simulator either, so the same override the M-Pesa rail
	// carries applies: a real handset must be charged a real, tiny amount.
	amountKES := req.AmountKES
	if a.cfg.PromptAmountKES > 0 {
		amountKES = int64(a.cfg.PromptAmountKES)
	}

	// The transaction id is ours to mint and must be unique per attempt:
	// resending one Airtel already holds is a duplicate-transaction error,
	// not a second prompt. The loan reference alone would repeat across
	// attempts, so the initiation second is appended.
	transactionID := fmt.Sprintf("%s-%d", reference, a.now().Unix())

	resp, err := a.client.Payment(ctx, airtel.PaymentRequest{
		Reference:     reference,
		Payer:         req.Payer,
		AmountKES:     amountKES,
		TransactionID: transactionID,
	})
	if err != nil {
		return nil, airtelErr("airtel_prompt", req.LoanID).Wrapf(err, "airtel push failed")
	}
	if !resp.Accepted() {
		return nil, airtelErr("airtel_prompt", req.LoanID).
			With("airtel_response_code", resp.ResponseCode()).
			Errorf("airtel declined the push")
	}

	// The payoff lock is req.AmountUSDCStroops, never derived from amountKES,
	// which may be the staging override — a fixed tiny figure with no
	// relationship to what the loan owes.
	//
	// The first enquiry is scheduled at Airtel's documented three-minute
	// floor, not at the shorter poll interval: asking sooner returns nothing
	// useful and spends a call.
	nextPoll := a.now().Add(a.enquiryDelay())
	lockedAt := a.now()
	if _, err := a.loanSvc.Update(ctx, req.LoanID, loan.UpdateLoanRequest{
		RepaymentProvider:      lo.ToPtr(models.LoanRepaymentProviderAirtel),
		RepaymentStatus:        lo.ToPtr(models.LoanRepaymentStatusInitiated),
		RepaymentAirtelTxnID:   lo.ToPtr(transactionID),
		RepaymentNextPollAt:    &nextPoll,
		RepaymentPayoffStroops: lo.ToPtr(req.AmountUSDCStroops),
		RepaymentLockedAt:      &lockedAt,
	}); err != nil {
		return nil, airtelErr("airtel_prompt", req.LoanID).Code(pkgErrors.CodeStateWriteFailed).
			Wrapf(err, "could not mark the repayment initiated")
	}

	return &cashin.PromptResult{
		LoanID:    req.LoanID,
		Reference: reference,
		Provider: airtel.PromptPayload{
			TransactionID: transactionID,
			Status:        airtel.ParseTransactionStatus(resp.Data.Transaction.Status),
			ResponseCode:  resp.ResponseCode(),
			PromptedKES:   amountKES,
		},
	}, nil
}

// Status resolves a transaction by enquiry, keyed by the id we minted.
//
// An enquiry reporting a failure is an answer, not an error: the Succeeded
// flag carries it, and only a failure to ask comes back as one.
func (a *AirtelCollectionAdapter) Status(ctx context.Context, ref cashin.ProviderRef) (*cashin.Status, error) {
	resp, err := a.client.Enquiry(ctx, ref.ID)
	if err != nil {
		return nil, airtelErr("airtel_status", "").Wrapf(err, "airtel enquiry failed")
	}

	receipt, _ := resp.Receipt()
	return &cashin.Status{
		Reference: ref.ID,
		Succeeded: resp.TransactionStatus().Succeeded(),
		Provider: airtel.CollectionPayload{
			AirtelMoneyID: receipt,
			TransactionID: ref.ID,
		},
	}, nil
}

// Reverse refunds a settled collection to the payer.
//
// ref.ID must be Airtel's own airtel_money_id, not the id we minted: refunds
// are keyed by the receipt and enquiries by our id, so a refund is impossible
// until an enquiry or a callback has disclosed it.
//
// Airtel supports full refunds only. The amount argument is not sent, and a
// caller must not read the result as a partial reversal.
func (a *AirtelCollectionAdapter) Reverse(ctx context.Context, ref cashin.ProviderRef, _ int64, _ string) (*cashin.ReversalResult, error) {
	if ref.ID == "" {
		return nil, airtelErr("airtel_reverse", "").
			Code(pkgErrors.CodeMissingDependency).
			Hint("Refunds take the id Airtel minted, which only an enquiry or a callback discloses.").
			Errorf("no airtel money id was supplied")
	}

	resp, err := a.client.Refund(ctx, ref.ID)
	if err != nil {
		return nil, airtelErr("airtel_reverse", "").Wrapf(err, "airtel refund failed")
	}

	status := airtel.ParseTransactionStatus(resp.Data.Transaction.Status)
	return &cashin.ReversalResult{
		Status:    string(status),
		Reference: ref.ID,
		Provider: airtel.RefundPayload{
			AirtelMoneyID: resp.Data.Transaction.AirtelMoneyID,
			Status:        status,
		},
	}, nil
}

// enquiryDelay is the wait before the first enquiry, never shorter than
// Airtel's documented floor.
func (a *AirtelCollectionAdapter) enquiryDelay() time.Duration {
	if a.cfg.EnquiryDelay <= 0 {
		return config.EnquiryDelayFloor
	}
	return a.cfg.EnquiryDelay
}
