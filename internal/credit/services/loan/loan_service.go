package loan

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
	"github.com/Shamba-Records-Limited/microvault/pkg/loanref"
)

// Valid repayment schedules
var validRepaymentSchedules = map[string]bool{
	"daily":     true,
	"weekly":    true,
	"bi_weekly": true,
	"monthly":   true,
	"lump_sum":  true,
}

// Service defines the interface for loan business logic operations
type Service interface {
	// Loan management
	Create(ctx context.Context, req CreateLoanRequest) (*LoanResponse, error)
	GetByID(ctx context.Context, id string) (*LoanResponse, error)
	GetByRampShortCode(ctx context.Context, code string) (*LoanResponse, error)
	GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[LoanResponse], error)
	GetActiveLoans(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[LoanResponse], error)
	Update(ctx context.Context, id string, req UpdateLoanRequest) (*LoanResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error

	// Loan lifecycle management
	Approve(ctx context.Context, id string, req ApproveLoanRequest) (*LoanResponse, error)
	Disburse(ctx context.Context, id string, req DisburseLoanRequest) (*LoanResponse, error)
	MarkAsRepaid(ctx context.Context, id string) (*LoanResponse, error)
	MarkAsDefaulted(ctx context.Context, id string) (*LoanResponse, error)
	MarkAsOffRampFailed(ctx context.Context, id string) (*LoanResponse, error)

	// Provider-scoped queries — used by the MoneyGram poller and the
	// dedupe gate in the loan service adapter.
	GetActiveByProvider(ctx context.Context, provider string, limit int) ([]*LoanResponse, error)
	GetActiveByUserAndProvider(ctx context.Context, userID, provider string) ([]*LoanResponse, error)
}

// service implements the Service interface
type service struct {
	repo      repository.LoanRepository
	refPrefix string
}

// NewService creates a new loan service instance
func NewService(repo repository.LoanRepository, refPrefix string) *service {
	return &service{
		repo:      repo,
		refPrefix: refPrefix,
	}
}

// referenceAttempts bounds the unique-index collision retry. At six random
// Crockford characters the space is 32^6 ≈ 1e9, so a collision is a rare event
// and a bounded retry turns it into a second attempt rather than a 500.
const referenceAttempts = 3

// Create creates a new loan with business validation
func (s *service) Create(ctx context.Context, req CreateLoanRequest) (*LoanResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Calculate due date
	dueDate := time.Now().AddDate(0, 0, req.DurationDays)

	// Only the origination fee is fixed at creation. Interest is not projected:
	// it accrues against the vault's borrow_index and the borrower is told to
	// check their balance, so a stored total would be a promise we do not make.
	originationFee := req.PrincipalAmount * int64(req.OriginationFeeBps) / 10000

	// Create loan model
	loan := &models.Loan{
		UserID:            req.UserID,
		AccountID:         req.AccountID,
		ProductID:         req.ProductID,
		PrincipalAmount:   req.PrincipalAmount,
		PrincipalAsset:    req.PrincipalAsset,
		VaultAPRBps:       req.VaultAPRBps,
		DurationDays:      req.DurationDays,
		RepaymentSchedule: req.RepaymentSchedule,
		DueDate:           &dueDate,
		Status:            models.LoanStatusPending,
	}
	if req.OriginationFeeBps > 0 {
		feeBps := req.OriginationFeeBps
		loan.OriginationFee = &originationFee
		loan.OriginationFeeBps = &feeBps
	}

	// Create the loan, generating a fresh reference on a unique conflict. The
	// model's BeforeCreate hook is the fallback for other creation paths; the
	// service sets the reference explicitly so the configured prefix applies.
	for attempt := 0; attempt < referenceAttempts; attempt++ {
		if loan.LoanReference == nil {
			ref, err := loanref.Generate(s.refPrefix)
			if err != nil {
				return nil, err
			}
			loan.LoanReference = &ref
		}
		if err := s.repo.Create(ctx, loan); err != nil {
			if errors.Is(err, repository.ErrLoanReferenceConflict) {
				loan.LoanReference = nil
				continue
			}
			slog.ErrorContext(ctx, "Create: failed to create loan", slog.Any("error", err))
			return nil, err
		}
		return toLoanResponse(loan), nil
	}

	return nil, repository.ErrLoanReferenceConflict
}

// GetByID retrieves a loan by ID
func (s *service) GetByID(ctx context.Context, id string) (*LoanResponse, error) {
	loan, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return nil, ErrLoanNotFound
		}
		slog.ErrorContext(ctx, "GetByID: failed to get loan", slog.Any("error", err))
		return nil, err
	}

	return toLoanResponse(loan), nil
}

// GetByRampShortCode resolves the loan behind a /r/{code} SMS redirect.
func (s *service) GetByRampShortCode(ctx context.Context, code string) (*LoanResponse, error) {
	loan, err := s.repo.GetByRampShortCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return nil, ErrLoanNotFound
		}
		slog.ErrorContext(ctx, "GetByRampShortCode: failed to get loan", slog.Any("error", err))
		return nil, err
	}

	return toLoanResponse(loan), nil
}

// GetByUserID retrieves loans by user ID with pagination
func (s *service) GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[LoanResponse], error) {
	if pagination.Page <= 0 {
		pagination.Page = 1
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	if pagination.PageSize > 100 {
		pagination.PageSize = 100
	}

	offset := (pagination.Page - 1) * pagination.PageSize

	loans, err := s.repo.GetByUserID(ctx, userID, pagination.PageSize, offset)
	if err != nil {
		slog.ErrorContext(ctx, "GetByUserID: failed to get loans", slog.Any("error", err))
		return nil, err
	}

	responses := make([]LoanResponse, len(loans))
	for i, loan := range loans {
		responses[i] = *toLoanResponse(loan)
	}

	return &services.PaginatedResponse[LoanResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetActiveLoans retrieves all active loans with pagination
func (s *service) GetActiveLoans(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[LoanResponse], error) {
	if pagination.Page <= 0 {
		pagination.Page = 1
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	if pagination.PageSize > 100 {
		pagination.PageSize = 100
	}

	offset := (pagination.Page - 1) * pagination.PageSize

	loans, err := s.repo.GetActiveLoans(ctx, pagination.PageSize, offset)
	if err != nil {
		slog.ErrorContext(ctx, "GetActiveLoans: failed to get loans", slog.Any("error", err))
		return nil, err
	}

	responses := make([]LoanResponse, len(loans))
	for i, loan := range loans {
		responses[i] = *toLoanResponse(loan)
	}

	return &services.PaginatedResponse[LoanResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// Update updates loan information
func (s *service) Update(ctx context.Context, id string, req UpdateLoanRequest) (*LoanResponse, error) {
	// Write only what the caller actually set. Rewriting every column made a
	// one-field change re-write the ~1KB SEP-24 URLs and touch all 20 indexes
	// on loans; the poller does this for every active loan every 30s.
	if fields := req.changedFields(); len(fields) > 0 {
		if err := s.repo.UpdateFields(ctx, id, fields); err != nil {
			if errors.Is(err, repository.ErrLoanNotFound) {
				return nil, ErrLoanNotFound
			}
			slog.ErrorContext(ctx, "Update: failed to update loan", slog.Any("error", err))
			return nil, err
		}
	}

	// Read back so the response reflects committed state rather than a
	// locally-mutated copy.
	loan, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return nil, ErrLoanNotFound
		}
		slog.ErrorContext(ctx, "Update: failed to get loan", slog.Any("error", err))
		return nil, err
	}

	return toLoanResponse(loan), nil
}

// Delete soft deletes a loan
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return ErrLoanNotFound
		}
		slog.ErrorContext(ctx, "Delete: failed to delete loan", slog.Any("error", err))
		return err
	}

	return nil
}

// Restore restores a soft-deleted loan
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return ErrLoanNotFound
		}
		slog.ErrorContext(ctx, "Restore: failed to restore loan", slog.Any("error", err))
		return err
	}

	return nil
}

// Approve approves a pending loan
func (s *service) Approve(ctx context.Context, id string, req ApproveLoanRequest) (*LoanResponse, error) {
	loan, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return nil, ErrLoanNotFound
		}
		slog.ErrorContext(ctx, "Approve: failed to get loan", slog.Any("error", err))
		return nil, err
	}

	if loan.Status != models.LoanStatusPending {
		return nil, ErrCannotApprovePendingLoan
	}

	now := time.Now()
	loan.Status = models.LoanStatusApproved
	loan.ApprovedAt = &now
	loan.ApprovedBy = &req.ApprovedBy

	if err := s.repo.Update(ctx, loan); err != nil {
		slog.ErrorContext(ctx, "Approve: failed to update loan", slog.Any("error", err))
		return nil, err
	}

	return toLoanResponse(loan), nil
}

// Disburse disburses an approved loan
func (s *service) Disburse(ctx context.Context, id string, req DisburseLoanRequest) (*LoanResponse, error) {
	loan, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return nil, ErrLoanNotFound
		}
		slog.ErrorContext(ctx, "Disburse: failed to get loan", slog.Any("error", err))
		return nil, err
	}

	if loan.Status != models.LoanStatusApproved {
		return nil, ErrCannotDisburseUnapproved
	}

	now := time.Now()
	loan.Status = models.LoanStatusDisbursing
	loan.DisbursedAt = &now

	// Update disbursement details
	if req.VaultTxHash != nil {
		loan.VaultTxHash = req.VaultTxHash
	}
	if req.VaultTxStatus != nil {
		loan.VaultTxStatus = req.VaultTxStatus
	}
	if req.RampProvider != nil {
		loan.RampProvider = req.RampProvider
	}
	if req.RampRequestID != nil {
		loan.RampRequestID = req.RampRequestID
	}
	if req.RampFiatAmount != nil {
		loan.RampFiatAmount = req.RampFiatAmount
	}
	if req.RampFiatCurr != nil {
		loan.RampFiatCurr = req.RampFiatCurr
	}
	if req.SettlementMethod != nil {
		loan.SettlementMethod = req.SettlementMethod
	}
	if req.RampSequenceID != nil {
		loan.RampSequenceID = req.RampSequenceID
	}

	if err := s.repo.Update(ctx, loan); err != nil {
		slog.ErrorContext(ctx, "Disburse: failed to update loan", slog.Any("error", err))
		return nil, err
	}

	return toLoanResponse(loan), nil
}

// MarkAsRepaid marks a loan as fully repaid
func (s *service) MarkAsRepaid(ctx context.Context, id string) (*LoanResponse, error) {
	loan, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return nil, ErrLoanNotFound
		}
		slog.ErrorContext(ctx, "MarkAsRepaid: failed to get loan", slog.Any("error", err))
		return nil, err
	}

	if loan.Status != models.LoanStatusDisbursed {
		return nil, ErrCannotRepayNonDisbursed
	}

	now := time.Now()
	loan.Status = models.LoanStatusRepaid
	loan.RepaidAt = &now

	if err := s.repo.Update(ctx, loan); err != nil {
		slog.ErrorContext(ctx, "MarkAsRepaid: failed to update loan", slog.Any("error", err))
		return nil, err
	}

	return toLoanResponse(loan), nil
}

// MarkAsDefaulted marks a loan as defaulted
func (s *service) MarkAsDefaulted(ctx context.Context, id string) (*LoanResponse, error) {
	loan, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return nil, ErrLoanNotFound
		}
		slog.ErrorContext(ctx, "MarkAsDefaulted: failed to get loan", slog.Any("error", err))
		return nil, err
	}

	if loan.Status != models.LoanStatusDisbursed {
		return nil, ErrCannotRepayNonDisbursed
	}

	now := time.Now()
	loan.Status = models.LoanStatusDefaulted
	loan.DefaultedAt = &now

	if err := s.repo.Update(ctx, loan); err != nil {
		slog.ErrorContext(ctx, "MarkAsDefaulted: failed to update loan", slog.Any("error", err))
		return nil, err
	}

	return toLoanResponse(loan), nil
}

// MarkAsOffRampFailed marks a loan as off-ramp-failed: vault borrow succeeded
// but fiat disbursement could not complete. Use after the borrowed USDC has
// been (or is being) repaid to the vault. The borrower owes nothing — this is
// distinct from LoanStatusDefaulted.
func (s *service) MarkAsOffRampFailed(ctx context.Context, id string) (*LoanResponse, error) {
	loan, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return nil, ErrLoanNotFound
		}
		slog.ErrorContext(ctx, "MarkAsOffRampFailed: failed to get loan", slog.Any("error", err))
		return nil, err
	}

	loan.Status = models.LoanStatusOffRampFailed

	if err := s.repo.Update(ctx, loan); err != nil {
		slog.ErrorContext(ctx, "MarkAsOffRampFailed: failed to update loan", slog.Any("error", err))
		return nil, err
	}

	return toLoanResponse(loan), nil
}

// GetActiveByProvider returns active (in-flight) loans for a given off-ramp
// provider (e.g. "moneygram"). Used by provider-specific pollers.
func (s *service) GetActiveByProvider(ctx context.Context, provider string, limit int) ([]*LoanResponse, error) {
	loans, err := s.repo.GetActiveByProvider(ctx, provider, limit, 0)
	if err != nil {
		slog.ErrorContext(ctx, "GetActiveByProvider", slog.Any("error", err))
		return nil, err
	}
	out := make([]*LoanResponse, 0, len(loans))
	for _, l := range loans {
		out = append(out, toLoanResponse(l))
	}
	return out, nil
}

// GetActiveByUserAndProvider returns active loans for a single user scoped
// to a provider. Used as the dedupe gate when initiating a new off-ramp.
func (s *service) GetActiveByUserAndProvider(ctx context.Context, userID, provider string) ([]*LoanResponse, error) {
	loans, err := s.repo.GetActiveByUserAndProvider(ctx, userID, provider)
	if err != nil {
		slog.ErrorContext(ctx, "GetActiveByUserAndProvider", slog.Any("error", err))
		return nil, err
	}
	out := make([]*LoanResponse, 0, len(loans))
	for _, l := range loans {
		out = append(out, toLoanResponse(l))
	}
	return out, nil
}

// --- Helper functions ---

// validateCreateRequest validates the create loan request
func (s *service) validateCreateRequest(req CreateLoanRequest) error {
	if req.UserID == "" || req.AccountID == "" {
		return ErrInvalidInput
	}

	if req.PrincipalAmount <= 0 {
		return ErrInvalidAmount
	}

	if req.PrincipalAsset == "" {
		return ErrInvalidInput
	}

	if req.VaultAPRBps <= 0 {
		return ErrInvalidInterestRate
	}

	if req.DurationDays <= 0 {
		return ErrInvalidDuration
	}

	if !validRepaymentSchedules[req.RepaymentSchedule] {
		return ErrInvalidRepaymentSchedule
	}

	return nil
}

// toLoanResponse converts a loan model to response DTO
func toLoanResponse(loan *models.Loan) *LoanResponse {
	return &LoanResponse{
		ID:                   loan.ID,
		LoanReference:        loan.LoanReference,
		UserID:               loan.UserID,
		AccountID:            loan.AccountID,
		ProductID:            loan.ProductID,
		PrincipalAmount:      loan.PrincipalAmount,
		PrincipalAsset:       loan.PrincipalAsset,
		VaultAPRBps:          loan.VaultAPRBps,
		OriginationFee:       loan.OriginationFee,
		OriginationFeeBps:    loan.OriginationFeeBps,
		DurationDays:         loan.DurationDays,
		RepaymentSched:       loan.RepaymentSchedule,
		DueDate:              loan.DueDate,
		Status:               loan.Status,
		ApprovedAt:           loan.ApprovedAt,
		ApprovedBy:           loan.ApprovedBy,
		DisbursedAt:          loan.DisbursedAt,
		RepaidAt:             loan.RepaidAt,
		DefaultedAt:          loan.DefaultedAt,
		VaultTxHash:          loan.VaultTxHash,
		VaultTxStatus:        loan.VaultTxStatus,
		VaultRepayTxHash:     loan.VaultRepayTxHash,
		VaultRepayStatus:     loan.VaultRepayStatus,
		RampProvider:         loan.RampProvider,
		RampRequestID:        loan.RampRequestID,
		RampFiatAmount:       loan.RampFiatAmount,
		RampFiatCurr:         loan.RampFiatCurr,
		SettlementMethod:     loan.SettlementMethod,
		DisbursementStatus:   ptrString(loan.DeriveDisbursementStatus()),
		RampSequenceID:       loan.RampSequenceID,
		DisbursementRate:     loan.DisbursementRate,
		DeliveredAmountLocal: loan.DeliveredAmountLocal,
		ConversionSpreadBps:  loan.ConversionSpreadBps,
		BorrowIndex:          loan.BorrowIndex,
		ServiceFeeUSD:        loan.ServiceFeeUSD,
		ServiceFeeLocal:      loan.ServiceFeeLocal,
		PartnerFeeUSD:        loan.PartnerFeeUSD,
		PartnerFeeLocal:      loan.PartnerFeeLocal,
		TelcoFeeUSD:          loan.TelcoFeeUSD,
		TelcoFeeLocal:        loan.TelcoFeeLocal,
		TaxUSD:               loan.TaxUSD,
		TaxLocal:             loan.TaxLocal,

		RampInteractiveURL:     loan.RampInteractiveURL,
		RampShortCode:          loan.RampShortCode,
		RampShortCodeExpiresAt: loan.RampShortCodeExpiresAt,
		RampMoreInfoShortCode:  loan.RampMoreInfoShortCode,
		RampExternalRef:        loan.RampExternalRef,
		RampMoreInfoURL:        loan.RampMoreInfoURL,
		RampChildAccountIndex:  loan.RampChildAccountIndex,
		RampStellarTxHash:      loan.RampStellarTxHash,
		EntryRateBuffered:      loan.EntryRateBuffered,
		EntryRateSource:        loan.EntryRateSource,
		EntryBufferBps:         loan.EntryBufferBps,
		RequestedLocalAmount:   loan.RequestedLocalAmount,
		RampWithdrawMemo:       loan.RampWithdrawMemo,
		RampWithdrawMemoType:   loan.RampWithdrawMemoType,

		RampRefundTxHash:    loan.RampRefundTxHash,
		RampRefundAmount:    loan.RampRefundAmount,
		RampRefundShortfall: loan.RampRefundShortfall,
		RampRefundedAt:      loan.RampRefundedAt,

		RepaymentStatus:          loan.RepaymentStatus,
		RepaymentPayoffStroops:   loan.RepaymentPayoffStroops,
		RepaymentLockedAt:        loan.RepaymentLockedAt,
		RepaymentExpiresAt:       loan.RepaymentExpiresAt,
		RepaymentMGTxID:          loan.RepaymentMGTxID,
		RepaymentNextPollAt:      loan.RepaymentNextPollAt,
		RepaymentReminderSentAt:  loan.RepaymentReminderSentAt,
		RepaymentVaultTxHash:     loan.RepaymentVaultTxHash,
		RepaymentVaultAttempts:   loan.RepaymentVaultAttempts,
		RepaymentReferenceSentAt: loan.RepaymentReferenceSentAt,
		RepaymentProvider:        loan.RepaymentProvider,
		RepaymentMpesaCheckoutID: loan.RepaymentMpesaCheckoutID,
		RepaymentMpesaTransID:    loan.RepaymentMpesaTransID,
		RepaymentSTKAttempts:     loan.RepaymentSTKAttempts,

		CreatedAt: loan.CreatedAt,
		UpdatedAt: loan.UpdatedAt,
	}
}

// ptrString is used for response fields that are computed rather than stored.
func ptrString(v string) *string { return &v }
