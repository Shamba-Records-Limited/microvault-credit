package loan

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
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
}

// service implements the Service interface
type service struct {
	repo repository.LoanRepository
}

// NewService creates a new loan service instance
func NewService(repo repository.LoanRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new loan with business validation
func (s *service) Create(ctx context.Context, req CreateLoanRequest) (*LoanResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Calculate due date
	dueDate := time.Now().AddDate(0, 0, req.DurationDays)

	// Create loan model
	loan := &models.Loan{
		UserID:            req.UserID,
		AccountID:         req.AccountID,
		ProductID:         req.ProductID,
		PrincipalAmount:   req.PrincipalAmount,
		PrincipalAsset:    req.PrincipalAsset,
		InterestRateBps:   req.InterestRateBps,
		DurationDays:      req.DurationDays,
		RepaymentSchedule: req.RepaymentSchedule,
		DueDate:           &dueDate,
		Status:            models.LoanStatusPending,
	}

	// Create loan in database
	if err := s.repo.Create(ctx, loan); err != nil {
		log.Printf("Create: failed to create loan: %v", err)
		return nil, err
	}

	return toLoanResponse(loan), nil
}

// GetByID retrieves a loan by ID
func (s *service) GetByID(ctx context.Context, id string) (*LoanResponse, error) {
	loan, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return nil, ErrLoanNotFound
		}
		log.Printf("GetByID: failed to get loan: %v", err)
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
		log.Printf("GetByUserID: failed to get loans: %v", err)
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
		log.Printf("GetActiveLoans: failed to get loans: %v", err)
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
	// Get existing loan
	loan, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanNotFound) {
			return nil, ErrLoanNotFound
		}
		log.Printf("Update: failed to get loan: %v", err)
		return nil, err
	}

	// Update fields
	if req.VaultTxHash != nil {
		loan.VaultTxHash = req.VaultTxHash
	}
	if req.VaultTxStatus != nil {
		loan.VaultTxStatus = req.VaultTxStatus
	}
	if req.VaultRepayTxHash != nil {
		loan.VaultRepayTxHash = req.VaultRepayTxHash
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
	if req.MomoProvider != nil {
		loan.MomoProvider = req.MomoProvider
	}
	if req.MomoTxID != nil {
		loan.MomoTxID = req.MomoTxID
	}
	if req.MomoStatus != nil {
		loan.MomoStatus = req.MomoStatus
	}
	if req.OriginationFee != nil {
		loan.OriginationFee = req.OriginationFee
	}
	if req.OriginationFeeBps != nil {
		loan.OriginationFeeBps = req.OriginationFeeBps
	}
	if req.TotalAmount != nil {
		loan.TotalAmount = req.TotalAmount
	}
	if req.SettlementMethod != nil {
		loan.SettlementMethod = req.SettlementMethod
	}
	if req.DisbursementStatus != nil {
		loan.DisbursementStatus = req.DisbursementStatus
	}
	if req.RampSequenceID != nil {
		loan.RampSequenceID = req.RampSequenceID
	}
	if req.DisbursementRateBps != nil {
		loan.DisbursementRateBps = req.DisbursementRateBps
	}
	if req.DisbursementAmtKES != nil {
		loan.DisbursementAmtKES = req.DisbursementAmtKES
	}
	if req.RepaymentAmtKES != nil {
		loan.RepaymentAmtKES = req.RepaymentAmtKES
	}
	if req.ConversionSpreadBps != nil {
		loan.ConversionSpreadBps = req.ConversionSpreadBps
	}
	if req.BorrowIndex != nil {
		loan.BorrowIndex = req.BorrowIndex
	}
	if req.RampFeeUSD != nil {
		loan.RampFeeUSD = req.RampFeeUSD
	}
	if req.RampFeeLocal != nil {
		loan.RampFeeLocal = req.RampFeeLocal
	}
	if req.RampInteractiveURL != nil {
		loan.RampInteractiveURL = req.RampInteractiveURL
	}
	if req.RampExternalRef != nil {
		loan.RampExternalRef = req.RampExternalRef
	}
	if req.RampMoreInfoURL != nil {
		loan.RampMoreInfoURL = req.RampMoreInfoURL
	}
	if req.RampChildAccountIndex != nil {
		loan.RampChildAccountIndex = req.RampChildAccountIndex
	}
	if req.EntryRateUsed != nil {
		loan.EntryRateUsed = req.EntryRateUsed
	}
	if req.EntryRateSource != nil {
		loan.EntryRateSource = req.EntryRateSource
	}
	if req.EntryBufferPct != nil {
		loan.EntryBufferPct = req.EntryBufferPct
	}
	if req.RequestedLocalAmount != nil {
		loan.RequestedLocalAmount = req.RequestedLocalAmount
	}

	// Update in database
	if err := s.repo.Update(ctx, loan); err != nil {
		log.Printf("Update: failed to update loan: %v", err)
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
		log.Printf("Delete: failed to delete loan: %v", err)
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
		log.Printf("Restore: failed to restore loan: %v", err)
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
		log.Printf("Approve: failed to get loan: %v", err)
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
		log.Printf("Approve: failed to update loan: %v", err)
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
		log.Printf("Disburse: failed to get loan: %v", err)
		return nil, err
	}

	if loan.Status != models.LoanStatusApproved {
		return nil, ErrCannotDisburseUnapproved
	}

	now := time.Now()
	loan.Status = models.LoanStatusDisbursed
	loan.DisbursedAt = &now

	// Update disbursement details
	if req.VaultTxHash != nil {
		loan.VaultTxHash = req.VaultTxHash
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
	if req.MomoProvider != nil {
		loan.MomoProvider = req.MomoProvider
	}
	if req.MomoTxID != nil {
		loan.MomoTxID = req.MomoTxID
	}
	if req.SettlementMethod != nil {
		loan.SettlementMethod = req.SettlementMethod
	}
	if req.DisbursementStatus != nil {
		loan.DisbursementStatus = req.DisbursementStatus
	}
	if req.RampSequenceID != nil {
		loan.RampSequenceID = req.RampSequenceID
	}

	if err := s.repo.Update(ctx, loan); err != nil {
		log.Printf("Disburse: failed to update loan: %v", err)
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
		log.Printf("MarkAsRepaid: failed to get loan: %v", err)
		return nil, err
	}

	if loan.Status != models.LoanStatusDisbursed {
		return nil, ErrCannotRepayNonDisbursed
	}

	now := time.Now()
	loan.Status = models.LoanStatusRepaid
	loan.RepaidAt = &now

	if err := s.repo.Update(ctx, loan); err != nil {
		log.Printf("MarkAsRepaid: failed to update loan: %v", err)
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
		log.Printf("MarkAsDefaulted: failed to get loan: %v", err)
		return nil, err
	}

	if loan.Status != models.LoanStatusDisbursed {
		return nil, ErrCannotRepayNonDisbursed
	}

	now := time.Now()
	loan.Status = models.LoanStatusDefaulted
	loan.DefaultedAt = &now

	if err := s.repo.Update(ctx, loan); err != nil {
		log.Printf("MarkAsDefaulted: failed to update loan: %v", err)
		return nil, err
	}

	return toLoanResponse(loan), nil
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

	if req.InterestRateBps <= 0 {
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
		ID:                  loan.ID,
		LoanNumber:          loan.LoanNumber,
		UserID:              loan.UserID,
		AccountID:           loan.AccountID,
		ProductID:           loan.ProductID,
		PrincipalAmount:     loan.PrincipalAmount,
		PrincipalAsset:      loan.PrincipalAsset,
		InterestRateBps:     loan.InterestRateBps,
		InterestAmount:      loan.InterestAmount,
		OriginationFee:      loan.OriginationFee,
		OriginationFeeBps:   loan.OriginationFeeBps,
		TotalAmount:         loan.TotalAmount,
		DurationDays:        loan.DurationDays,
		RepaymentSched:      loan.RepaymentSchedule,
		DueDate:             loan.DueDate,
		Status:              loan.Status,
		ApprovedAt:          loan.ApprovedAt,
		ApprovedBy:          loan.ApprovedBy,
		DisbursedAt:         loan.DisbursedAt,
		RepaidAt:            loan.RepaidAt,
		DefaultedAt:         loan.DefaultedAt,
		VaultTxHash:         loan.VaultTxHash,
		VaultTxStatus:       loan.VaultTxStatus,
		VaultRepayTxHash:    loan.VaultRepayTxHash,
		RampProvider:        loan.RampProvider,
		RampRequestID:       loan.RampRequestID,
		RampFiatAmount:      loan.RampFiatAmount,
		RampFiatCurr:        loan.RampFiatCurr,
		MomoProvider:        loan.MomoProvider,
		MomoTxID:            loan.MomoTxID,
		MomoStatus:          loan.MomoStatus,
		SettlementMethod:    loan.SettlementMethod,
		DisbursementStatus:  loan.DisbursementStatus,
		RampSequenceID:      loan.RampSequenceID,
		DisbursementRateBps: loan.DisbursementRateBps,
		DisbursementAmtKES:  loan.DisbursementAmtKES,
		RepaymentAmtKES:     loan.RepaymentAmtKES,
		ConversionSpreadBps: loan.ConversionSpreadBps,
		BorrowIndex:         loan.BorrowIndex,
		RampFeeUSD:          loan.RampFeeUSD,
		RampFeeLocal:        loan.RampFeeLocal,

		RampInteractiveURL:    loan.RampInteractiveURL,
		RampExternalRef:       loan.RampExternalRef,
		RampMoreInfoURL:       loan.RampMoreInfoURL,
		RampChildAccountIndex: loan.RampChildAccountIndex,
		EntryRateUsed:         loan.EntryRateUsed,
		EntryRateSource:       loan.EntryRateSource,
		EntryBufferPct:        loan.EntryBufferPct,
		RequestedLocalAmount:  loan.RequestedLocalAmount,

		CreatedAt:           loan.CreatedAt,
		UpdatedAt:           loan.UpdatedAt,
	}
}
