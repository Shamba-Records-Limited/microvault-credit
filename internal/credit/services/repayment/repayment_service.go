package repayment

import (
	"context"
	"errors"
	"log"
	"slices"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
)

// Valid repayment status transitions
var validStatusTransitions = map[string][]string{
	models.RepaymentStatusPending: {models.RepaymentStatusPaid, models.RepaymentStatusOverdue, models.RepaymentStatusPartial, models.RepaymentStatusWaived},
	models.RepaymentStatusOverdue: {models.RepaymentStatusPaid, models.RepaymentStatusPartial, models.RepaymentStatusWaived},
	models.RepaymentStatusPartial: {models.RepaymentStatusPaid, models.RepaymentStatusOverdue, models.RepaymentStatusWaived},
	models.RepaymentStatusPaid:    {}, // Cannot transition from paid
	models.RepaymentStatusWaived:  {}, // Cannot transition from waived
}

// Service defines the interface for repayment business logic operations
type Service interface {
	// Repayment management
	Create(ctx context.Context, req CreateRepaymentRequest) (*RepaymentResponse, error)
	BatchCreate(ctx context.Context, reqs []CreateRepaymentRequest) ([]*RepaymentResponse, error)
	GetByID(ctx context.Context, id string) (*RepaymentResponse, error)
	GetByLoanID(ctx context.Context, loanID string, pagination services.Pagination) (*services.PaginatedResponse[RepaymentResponse], error)
	GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[RepaymentResponse], error)
	GetOverdue(ctx context.Context, asOfDate time.Time, pagination services.Pagination) (*services.PaginatedResponse[RepaymentResponse], error)
	GetUpcoming(ctx context.Context, from, to time.Time, pagination services.Pagination) (*services.PaginatedResponse[RepaymentResponse], error)
	GetTotalAmountDue(ctx context.Context, loanID string) (int64, error)
	Update(ctx context.Context, id string, req UpdateRepaymentRequest) (*RepaymentResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error

	// Payment operations
	RecordPayment(ctx context.Context, id string, req RecordPaymentRequest) (*RepaymentResponse, error)
	WaiveRepayment(ctx context.Context, id string) (*RepaymentResponse, error)
	MarkAsOverdue(ctx context.Context, id string) (*RepaymentResponse, error)
}

// service implements the Service interface
type service struct {
	repo repository.RepaymentRepository
}

// NewService creates a new repayment service instance
func NewService(repo repository.RepaymentRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new repayment with business validation
func (s *service) Create(ctx context.Context, req CreateRepaymentRequest) (*RepaymentResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Create repayment model
	repayment := &models.Repayment{
		LoanID:            req.LoanID,
		UserID:            req.UserID,
		InstallmentNumber: req.InstallmentNumber,
		DueDate:           req.DueDate,
		AmountDue:         req.AmountDue,
		AmountPaid:        0,
		Status:            models.RepaymentStatusPending,
		LateFee:           0,
	}

	// Create repayment in database
	if err := s.repo.Create(ctx, repayment); err != nil {
		log.Printf("Create: failed to create repayment: %v", err)
		return nil, err
	}

	return toRepaymentResponse(repayment), nil
}

// BatchCreate creates multiple repayments with business validation
// Typically used for generating repayment schedules when a loan is disbursed
func (s *service) BatchCreate(ctx context.Context, reqs []CreateRepaymentRequest) ([]*RepaymentResponse, error) {
	if len(reqs) == 0 {
		return []*RepaymentResponse{}, nil
	}

	// Validate all requests first
	for i, req := range reqs {
		if err := s.validateCreateRequest(req); err != nil {
			log.Printf("BatchCreate: request %d invalid: %v", i, err)
			return nil, err
		}
	}

	// Validate installment numbers are sequential and unique
	installmentNumbers := make(map[int]bool)
	for i, req := range reqs {
		if installmentNumbers[req.InstallmentNumber] {
			log.Printf("BatchCreate: duplicate installment number %d at index %d", req.InstallmentNumber, i)
			return nil, ErrInvalidInput
		}
		installmentNumbers[req.InstallmentNumber] = true
	}

	// Convert to models
	repayments := make([]*models.Repayment, len(reqs))
	for i, req := range reqs {
		repayments[i] = &models.Repayment{
			LoanID:            req.LoanID,
			UserID:            req.UserID,
			InstallmentNumber: req.InstallmentNumber,
			DueDate:           req.DueDate,
			AmountDue:         req.AmountDue,
			AmountPaid:        0,
			Status:            models.RepaymentStatusPending,
			LateFee:           0,
		}
	}

	// Batch create in single transaction
	if err := s.repo.BatchCreate(ctx, repayments); err != nil {
		log.Printf("BatchCreate: failed to create %d repayments: %v", len(repayments), err)
		return nil, err
	}

	log.Printf("BatchCreate: successfully created %d repayments for loan", len(repayments))

	// Convert to responses
	responses := make([]*RepaymentResponse, len(repayments))
	for i, repayment := range repayments {
		responses[i] = toRepaymentResponse(repayment)
	}

	return responses, nil
}

// GetByID retrieves a repayment by ID
func (s *service) GetByID(ctx context.Context, id string) (*RepaymentResponse, error) {
	repayment, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrRepaymentNotFound) {
			return nil, ErrRepaymentNotFound
		}
		log.Printf("GetByID: failed to get repayment: %v", err)
		return nil, err
	}

	return toRepaymentResponse(repayment), nil
}

// GetByLoanID retrieves repayments by loan ID with pagination
func (s *service) GetByLoanID(ctx context.Context, loanID string, pagination services.Pagination) (*services.PaginatedResponse[RepaymentResponse], error) {
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

	repayments, err := s.repo.GetByLoanID(ctx, loanID, offset, pagination.PageSize)
	if err != nil {
		log.Printf("GetByLoanID: failed to get repayments: %v", err)
		return nil, err
	}

	responses := make([]RepaymentResponse, len(repayments))
	for i, repayment := range repayments {
		responses[i] = *toRepaymentResponse(repayment)
	}

	return &services.PaginatedResponse[RepaymentResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetByUserID retrieves repayments by user ID with pagination
func (s *service) GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[RepaymentResponse], error) {
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

	repayments, err := s.repo.GetByUserID(ctx, userID, offset, pagination.PageSize)
	if err != nil {
		log.Printf("GetByUserID: failed to get repayments: %v", err)
		return nil, err
	}

	responses := make([]RepaymentResponse, len(repayments))
	for i, repayment := range repayments {
		responses[i] = *toRepaymentResponse(repayment)
	}

	return &services.PaginatedResponse[RepaymentResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetOverdue retrieves overdue repayments with pagination
func (s *service) GetOverdue(ctx context.Context, asOfDate time.Time, pagination services.Pagination) (*services.PaginatedResponse[RepaymentResponse], error) {
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

	repayments, err := s.repo.GetOverdue(ctx, asOfDate, offset, pagination.PageSize)
	if err != nil {
		log.Printf("GetOverdue: failed to get repayments: %v", err)
		return nil, err
	}

	responses := make([]RepaymentResponse, len(repayments))
	for i, repayment := range repayments {
		responses[i] = *toRepaymentResponse(repayment)
	}

	return &services.PaginatedResponse[RepaymentResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetUpcoming retrieves upcoming repayments within a date range with pagination
func (s *service) GetUpcoming(ctx context.Context, from, to time.Time, pagination services.Pagination) (*services.PaginatedResponse[RepaymentResponse], error) {
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

	repayments, err := s.repo.GetUpcoming(ctx, from, to, offset, pagination.PageSize)
	if err != nil {
		log.Printf("GetUpcoming: failed to get repayments: %v", err)
		return nil, err
	}

	responses := make([]RepaymentResponse, len(repayments))
	for i, repayment := range repayments {
		responses[i] = *toRepaymentResponse(repayment)
	}

	return &services.PaginatedResponse[RepaymentResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetTotalAmountDue returns the total amount due for a loan
func (s *service) GetTotalAmountDue(ctx context.Context, loanID string) (int64, error) {
	totalDue, err := s.repo.GetTotalAmountDue(ctx, loanID)
	if err != nil {
		log.Printf("GetTotalAmountDue: failed to get total amount due: %v", err)
		return 0, err
	}

	return totalDue, nil
}

// Update updates repayment information
func (s *service) Update(ctx context.Context, id string, req UpdateRepaymentRequest) (*RepaymentResponse, error) {
	repayment, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrRepaymentNotFound) {
			return nil, ErrRepaymentNotFound
		}
		log.Printf("Update: failed to get repayment: %v", err)
		return nil, err
	}

	// Cannot modify paid or waived repayments
	if repayment.Status == models.RepaymentStatusPaid || repayment.Status == models.RepaymentStatusWaived {
		return nil, ErrCannotModifyPaidRepayment
	}

	// Update fields
	if req.AmountDue != nil {
		repayment.AmountDue = *req.AmountDue
	}
	if req.DueDate != nil {
		repayment.DueDate = *req.DueDate
	}
	if req.LateFee != nil {
		repayment.LateFee = *req.LateFee
	}

	// Update in database
	if err := s.repo.Update(ctx, repayment); err != nil {
		log.Printf("Update: failed to update repayment: %v", err)
		return nil, err
	}

	return toRepaymentResponse(repayment), nil
}

// Delete soft deletes a repayment
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrRepaymentNotFound) {
			return ErrRepaymentNotFound
		}
		log.Printf("Delete: failed to delete repayment: %v", err)
		return err
	}

	return nil
}

// Restore restores a soft-deleted repayment
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrRepaymentNotFound) {
			return ErrRepaymentNotFound
		}
		log.Printf("Restore: failed to restore repayment: %v", err)
		return err
	}

	return nil
}

// RecordPayment records a payment for a repayment
func (s *service) RecordPayment(ctx context.Context, id string, req RecordPaymentRequest) (*RepaymentResponse, error) {
	repayment, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrRepaymentNotFound) {
			return nil, ErrRepaymentNotFound
		}
		log.Printf("RecordPayment: failed to get repayment: %v", err)
		return nil, err
	}

	// Cannot pay already paid or waived repayments
	if repayment.Status == models.RepaymentStatusPaid {
		return nil, ErrRepaymentAlreadyPaid
	}
	if repayment.Status == models.RepaymentStatusWaived {
		return nil, ErrRepaymentAlreadyWaived
	}

	// Calculate total due including late fee
	totalDue := repayment.AmountDue + repayment.LateFee - repayment.AmountPaid

	// Update payment
	repayment.AmountPaid += req.AmountPaid
	repayment.PaymentMethod = req.PaymentMethod
	repayment.TransactionID = req.TransactionID

	// Determine status based on payment
	if repayment.AmountPaid >= totalDue {
		now := time.Now()
		repayment.Status = models.RepaymentStatusPaid
		repayment.PaidAt = &now
	} else {
		repayment.Status = models.RepaymentStatusPartial
	}

	// Update in database
	if err := s.repo.Update(ctx, repayment); err != nil {
		log.Printf("RecordPayment: failed to update repayment: %v", err)
		return nil, err
	}

	return toRepaymentResponse(repayment), nil
}

// WaiveRepayment waives a repayment
func (s *service) WaiveRepayment(ctx context.Context, id string) (*RepaymentResponse, error) {
	repayment, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrRepaymentNotFound) {
			return nil, ErrRepaymentNotFound
		}
		log.Printf("WaiveRepayment: failed to get repayment: %v", err)
		return nil, err
	}

	if repayment.Status == models.RepaymentStatusPaid {
		return nil, ErrRepaymentAlreadyPaid
	}
	if repayment.Status == models.RepaymentStatusWaived {
		return nil, ErrRepaymentAlreadyWaived
	}

	repayment.Status = models.RepaymentStatusWaived

	if err := s.repo.Update(ctx, repayment); err != nil {
		log.Printf("WaiveRepayment: failed to update repayment: %v", err)
		return nil, err
	}

	return toRepaymentResponse(repayment), nil
}

// MarkAsOverdue marks a repayment as overdue
func (s *service) MarkAsOverdue(ctx context.Context, id string) (*RepaymentResponse, error) {
	repayment, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrRepaymentNotFound) {
			return nil, ErrRepaymentNotFound
		}
		log.Printf("MarkAsOverdue: failed to get repayment: %v", err)
		return nil, err
	}

	if !s.isValidStatusTransition(repayment.Status, models.RepaymentStatusOverdue) {
		return nil, ErrInvalidStatusTransition
	}

	repayment.Status = models.RepaymentStatusOverdue

	if err := s.repo.Update(ctx, repayment); err != nil {
		log.Printf("MarkAsOverdue: failed to update repayment: %v", err)
		return nil, err
	}

	return toRepaymentResponse(repayment), nil
}

// --- Helper functions ---

// validateCreateRequest validates the create repayment request
func (s *service) validateCreateRequest(req CreateRepaymentRequest) error {
	if req.LoanID == "" || req.UserID == "" {
		return ErrInvalidInput
	}

	if req.InstallmentNumber <= 0 {
		return ErrInvalidInput
	}

	if req.AmountDue <= 0 {
		return ErrInvalidAmount
	}

	if req.DueDate.IsZero() {
		return ErrInvalidDueDate
	}

	return nil
}

// isValidStatusTransition checks if a status transition is valid
func (s *service) isValidStatusTransition(from, to string) bool {
	if from == to {
		return true
	}

	validTransitions, exists := validStatusTransitions[from]
	if !exists {
		return false
	}

	return slices.Contains(validTransitions, to)
}

// toRepaymentResponse converts a repayment model to response DTO
func toRepaymentResponse(repayment *models.Repayment) *RepaymentResponse {
	return &RepaymentResponse{
		ID:                repayment.ID,
		LoanID:            repayment.LoanID,
		UserID:            repayment.UserID,
		InstallmentNumber: repayment.InstallmentNumber,
		DueDate:           repayment.DueDate,
		AmountDue:         repayment.AmountDue,
		AmountPaid:        repayment.AmountPaid,
		PaidAt:            repayment.PaidAt,
		PaymentMethod:     repayment.PaymentMethod,
		Status:            repayment.Status,
		LateFee:           repayment.LateFee,
		TransactionID:     repayment.TransactionID,
		CreatedAt:         repayment.CreatedAt,
		UpdatedAt:         repayment.UpdatedAt,
	}
}
