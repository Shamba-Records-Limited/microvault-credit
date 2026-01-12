package creditscoringfactor

import (
	"context"
	"errors"
	"log"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
)

// Service defines the interface for credit scoring factor business logic operations
type Service interface {
	// Credit scoring factor management
	Create(ctx context.Context, req CreateCreditScoringFactorRequest) (*CreditScoringFactorResponse, error)
	BatchCreate(ctx context.Context, reqs []CreateCreditScoringFactorRequest) ([]*CreditScoringFactorResponse, error)
	GetByID(ctx context.Context, id string) (*CreditScoringFactorResponse, error)
	GetByName(ctx context.Context, name string) (*CreditScoringFactorResponse, error)
	GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[CreditScoringFactorResponse], error)
	GetActive(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[CreditScoringFactorResponse], error)
	Update(ctx context.Context, id string, req UpdateCreditScoringFactorRequest) (*CreditScoringFactorResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error

	// State management
	Activate(ctx context.Context, id string, activatedBy string) (*CreditScoringFactorResponse, error)
	Deactivate(ctx context.Context, id string, deactivatedBy string) (*CreditScoringFactorResponse, error)
}

// service implements the Service interface
type service struct {
	repo repository.CreditScoringFactorRepository
}

// NewService creates a new credit scoring factor service instance
func NewService(repo repository.CreditScoringFactorRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new credit scoring factor with business validation
func (s *service) Create(ctx context.Context, req CreateCreditScoringFactorRequest) (*CreditScoringFactorResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Check for duplicate name
	existing, err := s.repo.GetByName(ctx, req.FactorName)
	if err == nil && existing != nil {
		return nil, ErrFactorNameAlreadyExists
	}
	if err != nil && !errors.Is(err, repository.ErrCreditScoringFactorNotFound) {
		log.Printf("Create: failed to check duplicate name: %v", err)
		return nil, err
	}

	// Set default min data required
	minData := req.MinDataRequired
	if minData <= 0 {
		minData = 1
	}

	// Create credit scoring factor model
	factor := &models.CreditScoringFactor{
		FactorName:        req.FactorName,
		FactorDescription: req.FactorDescription,
		WeightBps:         req.WeightBps,
		IsActive:          req.IsActive,
		CalculationMethod: req.CalculationMethod,
		MinDataRequired:   minData,
		CreatedBy:         req.CreatedBy,
	}

	// Create factor in database
	if err := s.repo.Create(ctx, factor); err != nil {
		log.Printf("Create: failed to create credit scoring factor: %v", err)
		return nil, err
	}

	return toCreditScoringFactorResponse(factor), nil
}

// BatchCreate creates multiple credit scoring factors
func (s *service) BatchCreate(ctx context.Context, reqs []CreateCreditScoringFactorRequest) ([]*CreditScoringFactorResponse, error) {
	if len(reqs) == 0 {
		return []*CreditScoringFactorResponse{}, nil
	}

	factors := make([]*models.CreditScoringFactor, len(reqs))
	for i, req := range reqs {
		if err := s.validateCreateRequest(req); err != nil {
			return nil, err
		}

		minData := req.MinDataRequired
		if minData <= 0 {
			minData = 1
		}

		factors[i] = &models.CreditScoringFactor{
			FactorName:        req.FactorName,
			FactorDescription: req.FactorDescription,
			WeightBps:         req.WeightBps,
			IsActive:          req.IsActive,
			CalculationMethod: req.CalculationMethod,
			MinDataRequired:   minData,
			CreatedBy:         req.CreatedBy,
		}
	}

	if err := s.repo.BatchCreate(ctx, factors); err != nil {
		log.Printf("BatchCreate: failed to create factors: %v", err)
		return nil, err
	}

	responses := make([]*CreditScoringFactorResponse, len(factors))
	for i, factor := range factors {
		responses[i] = toCreditScoringFactorResponse(factor)
	}

	return responses, nil
}

// GetByID retrieves a credit scoring factor by ID
func (s *service) GetByID(ctx context.Context, id string) (*CreditScoringFactorResponse, error) {
	factor, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCreditScoringFactorNotFound) {
			return nil, ErrCreditScoringFactorNotFound
		}
		log.Printf("GetByID: failed to get credit scoring factor: %v", err)
		return nil, err
	}

	return toCreditScoringFactorResponse(factor), nil
}

// GetByName retrieves a credit scoring factor by name
func (s *service) GetByName(ctx context.Context, name string) (*CreditScoringFactorResponse, error) {
	factor, err := s.repo.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, repository.ErrCreditScoringFactorNotFound) {
			return nil, ErrCreditScoringFactorNotFound
		}
		log.Printf("GetByName: failed to get credit scoring factor: %v", err)
		return nil, err
	}

	return toCreditScoringFactorResponse(factor), nil
}

// GetAll retrieves all credit scoring factors with pagination
func (s *service) GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[CreditScoringFactorResponse], error) {
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

	factors, err := s.repo.GetAll(ctx, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetAll: failed to get credit scoring factors: %v", err)
		return nil, err
	}

	responses := make([]CreditScoringFactorResponse, len(factors))
	for i, factor := range factors {
		responses[i] = *toCreditScoringFactorResponse(factor)
	}

	return &services.PaginatedResponse[CreditScoringFactorResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetActive retrieves active credit scoring factors with pagination
func (s *service) GetActive(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[CreditScoringFactorResponse], error) {
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

	factors, err := s.repo.GetActive(ctx, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetActive: failed to get active credit scoring factors: %v", err)
		return nil, err
	}

	responses := make([]CreditScoringFactorResponse, len(factors))
	for i, factor := range factors {
		responses[i] = *toCreditScoringFactorResponse(factor)
	}

	return &services.PaginatedResponse[CreditScoringFactorResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// Update updates credit scoring factor information
func (s *service) Update(ctx context.Context, id string, req UpdateCreditScoringFactorRequest) (*CreditScoringFactorResponse, error) {
	factor, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCreditScoringFactorNotFound) {
			return nil, ErrCreditScoringFactorNotFound
		}
		log.Printf("Update: failed to get credit scoring factor: %v", err)
		return nil, err
	}

	// Update fields
	if req.FactorName != nil {
		// Check for duplicate name
		existing, err := s.repo.GetByName(ctx, *req.FactorName)
		if err == nil && existing != nil && existing.ID != id {
			return nil, ErrFactorNameAlreadyExists
		}
		factor.FactorName = *req.FactorName
	}
	if req.FactorDescription != nil {
		factor.FactorDescription = req.FactorDescription
	}
	if req.WeightBps != nil {
		if *req.WeightBps <= 0 {
			return nil, ErrInvalidWeight
		}
		factor.WeightBps = *req.WeightBps
	}
	if req.IsActive != nil {
		factor.IsActive = *req.IsActive
	}
	if req.CalculationMethod != nil {
		factor.CalculationMethod = req.CalculationMethod
	}
	if req.MinDataRequired != nil {
		factor.MinDataRequired = *req.MinDataRequired
	}
	if req.UpdatedBy != nil {
		factor.UpdatedBy = req.UpdatedBy
	}

	// Update in database
	if err := s.repo.Update(ctx, factor); err != nil {
		log.Printf("Update: failed to update credit scoring factor: %v", err)
		return nil, err
	}

	return toCreditScoringFactorResponse(factor), nil
}

// Delete soft deletes a credit scoring factor
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrCreditScoringFactorNotFound) {
			return ErrCreditScoringFactorNotFound
		}
		log.Printf("Delete: failed to delete credit scoring factor: %v", err)
		return err
	}

	return nil
}

// Restore restores a soft-deleted credit scoring factor
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrCreditScoringFactorNotFound) {
			return ErrCreditScoringFactorNotFound
		}
		log.Printf("Restore: failed to restore credit scoring factor: %v", err)
		return err
	}

	return nil
}

// Activate activates a credit scoring factor with business validation
func (s *service) Activate(ctx context.Context, id string, activatedBy string) (*CreditScoringFactorResponse, error) {
	// Get the factor
	factor, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCreditScoringFactorNotFound) {
			return nil, ErrCreditScoringFactorNotFound
		}
		log.Printf("Activate: failed to get credit scoring factor: %v", err)
		return nil, err
	}

	// Check if already active
	if factor.IsActive {
		return nil, ErrFactorAlreadyActive
	}

	// Activate the factor
	factor.IsActive = true
	if activatedBy != "" {
		factor.UpdatedBy = &activatedBy
	}

	// Update in database
	if err := s.repo.Update(ctx, factor); err != nil {
		log.Printf("Activate: failed to update credit scoring factor: %v", err)
		return nil, err
	}

	log.Printf("Activate: successfully activated credit scoring factor %s by %s", id, activatedBy)
	return toCreditScoringFactorResponse(factor), nil
}

// Deactivate deactivates a credit scoring factor with business validation
func (s *service) Deactivate(ctx context.Context, id string, deactivatedBy string) (*CreditScoringFactorResponse, error) {
	// Get the factor
	factor, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCreditScoringFactorNotFound) {
			return nil, ErrCreditScoringFactorNotFound
		}
		log.Printf("Deactivate: failed to get credit scoring factor: %v", err)
		return nil, err
	}

	// Check if already inactive
	if !factor.IsActive {
		return nil, ErrFactorAlreadyInactive
	}

	// Deactivate the factor
	factor.IsActive = false
	if deactivatedBy != "" {
		factor.UpdatedBy = &deactivatedBy
	}

	// Update in database
	if err := s.repo.Update(ctx, factor); err != nil {
		log.Printf("Deactivate: failed to update credit scoring factor: %v", err)
		return nil, err
	}

	log.Printf("Deactivate: successfully deactivated credit scoring factor %s by %s", id, deactivatedBy)
	return toCreditScoringFactorResponse(factor), nil
}

// --- Helper functions ---

// validateCreateRequest validates the create credit scoring factor request
func (s *service) validateCreateRequest(req CreateCreditScoringFactorRequest) error {
	if req.FactorName == "" {
		return ErrInvalidFactorName
	}

	if req.WeightBps <= 0 {
		return ErrInvalidWeight
	}

	return nil
}

// toCreditScoringFactorResponse converts a credit scoring factor model to response DTO
func toCreditScoringFactorResponse(factor *models.CreditScoringFactor) *CreditScoringFactorResponse {
	return &CreditScoringFactorResponse{
		ID:                factor.ID,
		FactorName:        factor.FactorName,
		FactorDescription: factor.FactorDescription,
		WeightBps:         factor.WeightBps,
		IsActive:          factor.IsActive,
		CalculationMethod: factor.CalculationMethod,
		MinDataRequired:   factor.MinDataRequired,
		CreatedAt:         factor.CreatedAt,
		UpdatedAt:         factor.UpdatedAt,
		CreatedBy:         factor.CreatedBy,
		UpdatedBy:         factor.UpdatedBy,
	}
}
