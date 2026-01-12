package creditfactor

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
)

// Service defines the interface for credit factor business logic operations
type Service interface {
	// Credit factor management
	Create(ctx context.Context, req CreateCreditFactorRequest) (*CreditFactorResponse, error)
	BatchCreate(ctx context.Context, reqs []CreateCreditFactorRequest) ([]*CreditFactorResponse, error)
	GetByID(ctx context.Context, id string) (*CreditFactorResponse, error)
	GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[CreditFactorResponse], error)
	GetByCreditScoreID(ctx context.Context, creditScoreID string, pagination services.Pagination) (*services.PaginatedResponse[CreditFactorResponse], error)
	GetLatestByUserID(ctx context.Context, userID string) ([]*CreditFactorResponse, error)
	GetByUserAndFactorName(ctx context.Context, userID, factorName string) (*CreditFactorResponse, error)
	Update(ctx context.Context, id string, req UpdateCreditFactorRequest) (*CreditFactorResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error

	// Aggregations
	GetTotalWeightedScoreByUser(ctx context.Context, userID string) (int32, error)
	GetAverageFactorValueByName(ctx context.Context, factorName string) (int32, error)
	CountByUserID(ctx context.Context, userID string) (int64, error)
}

// service implements the Service interface
type service struct {
	repo repository.CreditFactorRepository
}

// NewService creates a new credit factor service instance
func NewService(repo repository.CreditFactorRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new credit factor with business validation
func (s *service) Create(ctx context.Context, req CreateCreditFactorRequest) (*CreditFactorResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Create credit factor model
	factor := &models.CreditFactor{
		UserID:            req.UserID,
		CreditScoreID:     req.CreditScoreID,
		FactorName:        req.FactorName,
		FactorValueBps:    req.FactorValueBps,
		FactorWeightBps:   req.FactorWeightBps,
		CalculationMethod: req.CalculationMethod,
		DataSource:        req.DataSource,
		CalculatedAt:      time.Now(),
	}

	// Create factor in database (repository handles weighted score calculation)
	if err := s.repo.Create(ctx, factor); err != nil {
		log.Printf("Create: failed to create credit factor: %v", err)
		return nil, err
	}

	return toCreditFactorResponse(factor), nil
}

// BatchCreate creates multiple credit factors
func (s *service) BatchCreate(ctx context.Context, reqs []CreateCreditFactorRequest) ([]*CreditFactorResponse, error) {
	if len(reqs) == 0 {
		return []*CreditFactorResponse{}, nil
	}

	factors := make([]*models.CreditFactor, len(reqs))
	for i, req := range reqs {
		if err := s.validateCreateRequest(req); err != nil {
			return nil, err
		}

		factors[i] = &models.CreditFactor{
			UserID:            req.UserID,
			CreditScoreID:     req.CreditScoreID,
			FactorName:        req.FactorName,
			FactorValueBps:    req.FactorValueBps,
			FactorWeightBps:   req.FactorWeightBps,
			CalculationMethod: req.CalculationMethod,
			DataSource:        req.DataSource,
			CalculatedAt:      time.Now(),
		}
	}

	if err := s.repo.BatchCreate(ctx, factors); err != nil {
		log.Printf("BatchCreate: failed to create factors: %v", err)
		return nil, err
	}

	responses := make([]*CreditFactorResponse, len(factors))
	for i, factor := range factors {
		responses[i] = toCreditFactorResponse(factor)
	}

	return responses, nil
}

// GetByID retrieves a credit factor by ID
func (s *service) GetByID(ctx context.Context, id string) (*CreditFactorResponse, error) {
	factor, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCreditFactorNotFound) {
			return nil, ErrCreditFactorNotFound
		}
		log.Printf("GetByID: failed to get credit factor: %v", err)
		return nil, err
	}

	return toCreditFactorResponse(factor), nil
}

// GetByUserID retrieves credit factors by user ID with pagination
func (s *service) GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[CreditFactorResponse], error) {
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

	factors, err := s.repo.GetByUserID(ctx, userID, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByUserID: failed to get credit factors: %v", err)
		return nil, err
	}

	responses := make([]CreditFactorResponse, len(factors))
	for i, factor := range factors {
		responses[i] = *toCreditFactorResponse(factor)
	}

	return &services.PaginatedResponse[CreditFactorResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetByCreditScoreID retrieves credit factors by credit score ID with pagination
func (s *service) GetByCreditScoreID(ctx context.Context, creditScoreID string, pagination services.Pagination) (*services.PaginatedResponse[CreditFactorResponse], error) {
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

	factors, err := s.repo.GetByCreditScoreID(ctx, creditScoreID, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByCreditScoreID: failed to get credit factors: %v", err)
		return nil, err
	}

	responses := make([]CreditFactorResponse, len(factors))
	for i, factor := range factors {
		responses[i] = *toCreditFactorResponse(factor)
	}

	return &services.PaginatedResponse[CreditFactorResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetLatestByUserID retrieves the most recent credit factors for a user
func (s *service) GetLatestByUserID(ctx context.Context, userID string) ([]*CreditFactorResponse, error) {
	factors, err := s.repo.GetLatestByUserID(ctx, userID)
	if err != nil {
		log.Printf("GetLatestByUserID: failed to get credit factors: %v", err)
		return nil, err
	}

	responses := make([]*CreditFactorResponse, len(factors))
	for i, factor := range factors {
		responses[i] = toCreditFactorResponse(factor)
	}

	return responses, nil
}

// GetByUserAndFactorName retrieves a specific factor type for a user
func (s *service) GetByUserAndFactorName(ctx context.Context, userID, factorName string) (*CreditFactorResponse, error) {
	factor, err := s.repo.GetByUserAndFactorName(ctx, userID, factorName)
	if err != nil {
		if errors.Is(err, repository.ErrCreditFactorNotFound) {
			return nil, ErrCreditFactorNotFound
		}
		log.Printf("GetByUserAndFactorName: failed to get credit factor: %v", err)
		return nil, err
	}

	return toCreditFactorResponse(factor), nil
}

// Update updates credit factor information
func (s *service) Update(ctx context.Context, id string, req UpdateCreditFactorRequest) (*CreditFactorResponse, error) {
	factor, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCreditFactorNotFound) {
			return nil, ErrCreditFactorNotFound
		}
		log.Printf("Update: failed to get credit factor: %v", err)
		return nil, err
	}

	// Update fields
	if req.FactorValueBps != nil {
		factor.FactorValueBps = *req.FactorValueBps
	}
	if req.FactorWeightBps != nil {
		factor.FactorWeightBps = *req.FactorWeightBps
	}
	if req.CalculationMethod != nil {
		factor.CalculationMethod = req.CalculationMethod
	}
	if req.DataSource != nil {
		factor.DataSource = req.DataSource
	}

	// Update in database (repository handles recalculations)
	if err := s.repo.Update(ctx, factor); err != nil {
		log.Printf("Update: failed to update credit factor: %v", err)
		return nil, err
	}

	return toCreditFactorResponse(factor), nil
}

// Delete soft deletes a credit factor
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrCreditFactorNotFound) {
			return ErrCreditFactorNotFound
		}
		log.Printf("Delete: failed to delete credit factor: %v", err)
		return err
	}

	return nil
}

// Restore restores a soft-deleted credit factor
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrCreditFactorNotFound) {
			return ErrCreditFactorNotFound
		}
		log.Printf("Restore: failed to restore credit factor: %v", err)
		return err
	}

	return nil
}

// DeleteByUserID deletes all credit factors for a user
func (s *service) DeleteByUserID(ctx context.Context, userID string) error {
	if err := s.repo.DeleteByUserID(ctx, userID); err != nil {
		log.Printf("DeleteByUserID: failed to delete credit factors: %v", err)
		return err
	}

	return nil
}

// GetTotalWeightedScoreByUser returns total weighted score for a user
func (s *service) GetTotalWeightedScoreByUser(ctx context.Context, userID string) (int32, error) {
	total, err := s.repo.GetTotalWeightedScoreByUser(ctx, userID)
	if err != nil {
		log.Printf("GetTotalWeightedScoreByUser: failed to get total: %v", err)
		return 0, err
	}
	return total, nil
}

// GetAverageFactorValueByName returns average value for a specific factor type
func (s *service) GetAverageFactorValueByName(ctx context.Context, factorName string) (int32, error) {
	avg, err := s.repo.GetAverageFactorValueByName(ctx, factorName)
	if err != nil {
		log.Printf("GetAverageFactorValueByName: failed to get average: %v", err)
		return 0, err
	}
	return avg, nil
}

// CountByUserID counts the number of credit factors for a user
func (s *service) CountByUserID(ctx context.Context, userID string) (int64, error) {
	count, err := s.repo.CountByUserID(ctx, userID)
	if err != nil {
		log.Printf("CountByUserID: failed to count factors: %v", err)
		return 0, err
	}
	return count, nil
}

// --- Helper functions ---

// validateCreateRequest validates the create credit factor request
func (s *service) validateCreateRequest(req CreateCreditFactorRequest) error {
	if req.UserID == "" {
		return ErrInvalidInput
	}

	if req.FactorName == "" {
		return ErrInvalidFactorName
	}

	return nil
}

// toCreditFactorResponse converts a credit factor model to response DTO
func toCreditFactorResponse(factor *models.CreditFactor) *CreditFactorResponse {
	return &CreditFactorResponse{
		ID:                factor.ID,
		UserID:            factor.UserID,
		CreditScoreID:     factor.CreditScoreID,
		FactorName:        factor.FactorName,
		FactorValueBps:    factor.FactorValueBps,
		FactorWeightBps:   factor.FactorWeightBps,
		WeightedScoreBps:  factor.WeightedScoreBps,
		CalculationMethod: factor.CalculationMethod,
		DataSource:        factor.DataSource,
		CalculatedAt:      factor.CalculatedAt,
		CreatedAt:         factor.CreatedAt,
		UpdatedAt:         factor.UpdatedAt,
	}
}
