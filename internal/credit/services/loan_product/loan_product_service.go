package loanproduct

import (
	"context"
	"errors"
	"log"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
)

// Valid interest types
var validInterestTypes = map[string]bool{
	models.InterestTypeSimple:   true,
	models.InterestTypeCompound: true,
}

// Service defines the interface for loan product business logic operations
type Service interface {
	// Loan product management
	Create(ctx context.Context, req CreateLoanProductRequest) (*LoanProductResponse, error)
	GetByID(ctx context.Context, id string) (*LoanProductResponse, error)
	GetByName(ctx context.Context, name string) (*LoanProductResponse, error)
	GetActive(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[LoanProductResponse], error)
	GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[LoanProductResponse], error)
	Update(ctx context.Context, id string, req UpdateLoanProductRequest) (*LoanProductResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error
}

// service implements the Service interface
type service struct {
	repo repository.LoanProductRepository
}

// NewService creates a new loan product service instance
func NewService(repo repository.LoanProductRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new loan product with business validation
func (s *service) Create(ctx context.Context, req CreateLoanProductRequest) (*LoanProductResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Note: Duplicate name check skipped as repository doesn't have GetByName method
	// This should be enforced at the database level with a unique constraint

	// Create loan product model
	product := &models.LoanProduct{
		Name:                      req.Name,
		Description:               req.Description,
		InterestRateBps:           req.InterestRateBps,
		InterestType:              req.InterestType,
		OriginationFeeBps:         req.OriginationFeeBps,
		MinAmount:                 req.MinAmount,
		MaxAmount:                 req.MaxAmount,
		MinDurationDays:           req.MinDurationDays,
		MaxDurationDays:           req.MaxDurationDays,
		AllowedRepaymentSchedules: req.AllowedRepaymentSchedules,
		MaxCreditMultiplierBps:    req.MaxCreditMultiplierBps,
		RequiresCollateral:        req.RequiresCollateral,
		CollateralBps:             req.CollateralBps,
		PriorityOrder:             req.PriorityOrder,
		IsActive:                  req.IsActive,
	}

	// Create loan product in database
	if err := s.repo.Create(ctx, product); err != nil {
		log.Printf("Create: failed to create loan product: %v", err)
		return nil, err
	}

	return toLoanProductResponse(product), nil
}

// GetByID retrieves a loan product by ID
func (s *service) GetByID(ctx context.Context, id string) (*LoanProductResponse, error) {
	product, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanProductNotFound) {
			return nil, ErrLoanProductNotFound
		}
		log.Printf("GetByID: failed to get loan product: %v", err)
		return nil, err
	}

	return toLoanProductResponse(product), nil
}

// GetByName retrieves a loan product by name
// Note: This method is not fully implemented as the repository lacks GetByName
func (s *service) GetByName(ctx context.Context, name string) (*LoanProductResponse, error) {
	// Get all products and filter by name
	// This is inefficient and should be fixed by adding GetByName to the repository
	products, err := s.repo.GetAll(ctx, 1000, 0)
	if err != nil {
		log.Printf("GetByName: failed to get loan products: %v", err)
		return nil, err
	}

	for _, product := range products {
		if product.Name == name {
			return toLoanProductResponse(product), nil
		}
	}

	return nil, ErrLoanProductNotFound
}

// GetActive retrieves all active loan products with pagination
func (s *service) GetActive(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[LoanProductResponse], error) {
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

	products, err := s.repo.GetAllActive(ctx, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetActive: failed to get loan products: %v", err)
		return nil, err
	}

	responses := make([]LoanProductResponse, len(products))
	for i, product := range products {
		responses[i] = *toLoanProductResponse(product)
	}

	return &services.PaginatedResponse[LoanProductResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetAll retrieves all loan products with pagination
func (s *service) GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[LoanProductResponse], error) {
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

	products, err := s.repo.GetAll(ctx, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetAll: failed to get loan products: %v", err)
		return nil, err
	}

	responses := make([]LoanProductResponse, len(products))
	for i, product := range products {
		responses[i] = *toLoanProductResponse(product)
	}

	return &services.PaginatedResponse[LoanProductResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// Update updates loan product information
func (s *service) Update(ctx context.Context, id string, req UpdateLoanProductRequest) (*LoanProductResponse, error) {
	product, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanProductNotFound) {
			return nil, ErrLoanProductNotFound
		}
		log.Printf("Update: failed to get loan product: %v", err)
		return nil, err
	}

	// Update fields
	if req.Name != nil {
		// Note: Duplicate name check skipped as repository doesn't have GetByName
		// This should be enforced at the database level with a unique constraint
		product.Name = *req.Name
	}
	if req.Description != nil {
		product.Description = req.Description
	}
	if req.InterestRateBps != nil {
		if *req.InterestRateBps <= 0 {
			return nil, ErrInvalidInterestRate
		}
		product.InterestRateBps = *req.InterestRateBps
	}
	if req.InterestType != nil {
		if !validInterestTypes[*req.InterestType] {
			return nil, ErrInvalidInterestType
		}
		product.InterestType = *req.InterestType
	}
	if req.OriginationFeeBps != nil {
		product.OriginationFeeBps = req.OriginationFeeBps
	}
	if req.MinAmount != nil {
		product.MinAmount = *req.MinAmount
	}
	if req.MaxAmount != nil {
		product.MaxAmount = *req.MaxAmount
	}
	if req.MinDurationDays != nil {
		product.MinDurationDays = *req.MinDurationDays
	}
	if req.MaxDurationDays != nil {
		product.MaxDurationDays = *req.MaxDurationDays
	}
	if req.AllowedRepaymentSchedules != nil {
		product.AllowedRepaymentSchedules = req.AllowedRepaymentSchedules
	}
	if req.MaxCreditMultiplierBps != nil {
		product.MaxCreditMultiplierBps = *req.MaxCreditMultiplierBps
	}
	if req.RequiresCollateral != nil {
		product.RequiresCollateral = *req.RequiresCollateral
	}
	if req.CollateralBps != nil {
		product.CollateralBps = req.CollateralBps
	}
	if req.PriorityOrder != nil {
		product.PriorityOrder = *req.PriorityOrder
	}
	if req.IsActive != nil {
		product.IsActive = *req.IsActive
	}

	// Validate updated ranges
	if product.MaxAmount <= product.MinAmount {
		return nil, ErrInvalidAmountRange
	}
	if product.MaxDurationDays <= product.MinDurationDays {
		return nil, ErrInvalidDurationRange
	}

	// Update in database
	if err := s.repo.Update(ctx, product); err != nil {
		log.Printf("Update: failed to update loan product: %v", err)
		return nil, err
	}

	return toLoanProductResponse(product), nil
}

// Delete soft deletes a loan product
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrLoanProductNotFound) {
			return ErrLoanProductNotFound
		}
		log.Printf("Delete: failed to delete loan product: %v", err)
		return err
	}

	return nil
}

// Restore restores a soft-deleted loan product
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrLoanProductNotFound) {
			return ErrLoanProductNotFound
		}
		log.Printf("Restore: failed to restore loan product: %v", err)
		return err
	}

	return nil
}

// --- Helper functions ---

// validateCreateRequest validates the create loan product request
func (s *service) validateCreateRequest(req CreateLoanProductRequest) error {
	if req.Name == "" {
		return ErrInvalidProductName
	}

	if req.InterestRateBps <= 0 {
		return ErrInvalidInterestRate
	}

	if !validInterestTypes[req.InterestType] {
		return ErrInvalidInterestType
	}

	if req.MinAmount <= 0 {
		return ErrInvalidMinAmount
	}

	if req.MaxAmount <= 0 {
		return ErrInvalidMaxAmount
	}

	if req.MaxAmount <= req.MinAmount {
		return ErrInvalidAmountRange
	}

	if req.MinDurationDays <= 0 {
		return ErrInvalidMinDuration
	}

	if req.MaxDurationDays <= 0 {
		return ErrInvalidMaxDuration
	}

	if req.MaxDurationDays <= req.MinDurationDays {
		return ErrInvalidDurationRange
	}

	if len(req.AllowedRepaymentSchedules) == 0 {
		return ErrInvalidInput
	}

	return nil
}

// toLoanProductResponse converts a loan product model to response DTO
func toLoanProductResponse(product *models.LoanProduct) *LoanProductResponse {
	return &LoanProductResponse{
		ID:                        product.ID,
		Name:                      product.Name,
		Description:               product.Description,
		InterestRateBps:           product.InterestRateBps,
		InterestType:              product.InterestType,
		OriginationFeeBps:         product.OriginationFeeBps,
		MinAmount:                 product.MinAmount,
		MaxAmount:                 product.MaxAmount,
		MinDurationDays:           product.MinDurationDays,
		MaxDurationDays:           product.MaxDurationDays,
		AllowedRepaymentSchedules: product.AllowedRepaymentSchedules,
		MaxCreditMultiplierBps:    product.MaxCreditMultiplierBps,
		RequiresCollateral:        product.RequiresCollateral,
		CollateralBps:             product.CollateralBps,
		PriorityOrder:             product.PriorityOrder,
		IsActive:                  product.IsActive,
		CreatedAt:                 product.CreatedAt,
		UpdatedAt:                 product.UpdatedAt,
	}
}
