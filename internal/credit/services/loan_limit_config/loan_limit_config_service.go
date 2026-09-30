package loanlimitconfig

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
)

// Service defines the interface for loan limit config business logic operations
type Service interface {
	// Loan limit config management
	Create(ctx context.Context, req CreateLoanLimitConfigRequest) (*LoanLimitConfigResponse, error)
	BatchCreate(ctx context.Context, reqs []CreateLoanLimitConfigRequest) ([]*LoanLimitConfigResponse, error)
	GetByID(ctx context.Context, id string) (*LoanLimitConfigResponse, error)
	GetByRiskTier(ctx context.Context, riskTier string) (*LoanLimitConfigResponse, error)
	GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[LoanLimitConfigResponse], error)
	GetActive(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[LoanLimitConfigResponse], error)
	Update(ctx context.Context, id string, req UpdateLoanLimitConfigRequest) (*LoanLimitConfigResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error

	// State management
	Activate(ctx context.Context, id string, activatedBy string) (*LoanLimitConfigResponse, error)
	Deactivate(ctx context.Context, id string, deactivatedBy string) (*LoanLimitConfigResponse, error)
}

// service implements the Service interface
type service struct {
	repo repository.LoanLimitConfigRepository
}

// NewService creates a new loan limit config service instance
func NewService(repo repository.LoanLimitConfigRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new loan limit config with business validation
func (s *service) Create(ctx context.Context, req CreateLoanLimitConfigRequest) (*LoanLimitConfigResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Check for duplicate risk tier
	existing, err := s.repo.GetByRiskTier(ctx, req.RiskTier)
	if err == nil && existing != nil {
		return nil, ErrRiskTierAlreadyExists
	}
	if err != nil && !errors.Is(err, repository.ErrLoanLimitConfigNotFound) {
		slog.ErrorContext(ctx, "Create: failed to check duplicate risk tier", slog.Any("error", err))
		return nil, err
	}

	// Create loan limit config model
	config := &models.LoanLimitConfig{
		RiskTier:            req.RiskTier,
		MinLoanAmount:       req.MinLoanAmount,
		MaxLoanAmount:       req.MaxLoanAmount,
		IncomeMultiplierBps: req.IncomeMultiplierBps,
		MaxConcurrentLoans:  req.MaxConcurrentLoans,
		MaxLoanDurationDays: req.MaxLoanDurationDays,
		InterestRateBps:     req.InterestRateBps,
		LateFeeBps:          req.LateFeeBps,
		DefaultPenaltyBps:   req.DefaultPenaltyBps,
		IsActive:            req.IsActive,
		CreatedBy:           req.CreatedBy,
	}

	// Create config in database
	if err := s.repo.Create(ctx, config); err != nil {
		slog.ErrorContext(ctx, "Create: failed to create loan limit config", slog.Any("error", err))
		return nil, err
	}

	return toLoanLimitConfigResponse(config), nil
}

// BatchCreate creates multiple loan limit configs
func (s *service) BatchCreate(ctx context.Context, reqs []CreateLoanLimitConfigRequest) ([]*LoanLimitConfigResponse, error) {
	if len(reqs) == 0 {
		return []*LoanLimitConfigResponse{}, nil
	}

	configs := make([]*models.LoanLimitConfig, len(reqs))
	for i, req := range reqs {
		if err := s.validateCreateRequest(req); err != nil {
			return nil, err
		}

		configs[i] = &models.LoanLimitConfig{
			RiskTier:            req.RiskTier,
			MinLoanAmount:       req.MinLoanAmount,
			MaxLoanAmount:       req.MaxLoanAmount,
			IncomeMultiplierBps: req.IncomeMultiplierBps,
			MaxConcurrentLoans:  req.MaxConcurrentLoans,
			MaxLoanDurationDays: req.MaxLoanDurationDays,
			InterestRateBps:     req.InterestRateBps,
			LateFeeBps:          req.LateFeeBps,
			DefaultPenaltyBps:   req.DefaultPenaltyBps,
			IsActive:            req.IsActive,
			CreatedBy:           req.CreatedBy,
		}
	}

	if err := s.repo.BatchCreate(ctx, configs); err != nil {
		slog.ErrorContext(ctx, "BatchCreate: failed to create configs", slog.Any("error", err))
		return nil, err
	}

	responses := make([]*LoanLimitConfigResponse, len(configs))
	for i, config := range configs {
		responses[i] = toLoanLimitConfigResponse(config)
	}

	return responses, nil
}

// GetByID retrieves a loan limit config by ID
func (s *service) GetByID(ctx context.Context, id string) (*LoanLimitConfigResponse, error) {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanLimitConfigNotFound) {
			return nil, ErrLoanLimitConfigNotFound
		}
		slog.ErrorContext(ctx, "GetByID: failed to get loan limit config", slog.Any("error", err))
		return nil, err
	}

	return toLoanLimitConfigResponse(config), nil
}

// GetByRiskTier retrieves a loan limit config by risk tier
func (s *service) GetByRiskTier(ctx context.Context, riskTier string) (*LoanLimitConfigResponse, error) {
	config, err := s.repo.GetByRiskTier(ctx, riskTier)
	if err != nil {
		if errors.Is(err, repository.ErrLoanLimitConfigNotFound) {
			return nil, ErrLoanLimitConfigNotFound
		}
		slog.ErrorContext(ctx, "GetByRiskTier: failed to get loan limit config", slog.Any("error", err))
		return nil, err
	}

	return toLoanLimitConfigResponse(config), nil
}

// GetAll retrieves all loan limit configs with pagination
func (s *service) GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[LoanLimitConfigResponse], error) {
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

	configs, err := s.repo.GetAll(ctx, pagination.PageSize, offset)
	if err != nil {
		slog.ErrorContext(ctx, "GetAll: failed to get loan limit configs", slog.Any("error", err))
		return nil, err
	}

	responses := make([]LoanLimitConfigResponse, len(configs))
	for i, config := range configs {
		responses[i] = *toLoanLimitConfigResponse(config)
	}

	return &services.PaginatedResponse[LoanLimitConfigResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetActive retrieves active loan limit configs with pagination
func (s *service) GetActive(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[LoanLimitConfigResponse], error) {
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

	configs, err := s.repo.GetActive(ctx, pagination.PageSize, offset)
	if err != nil {
		slog.ErrorContext(ctx, "GetActive: failed to get active loan limit configs", slog.Any("error", err))
		return nil, err
	}

	responses := make([]LoanLimitConfigResponse, len(configs))
	for i, config := range configs {
		responses[i] = *toLoanLimitConfigResponse(config)
	}

	return &services.PaginatedResponse[LoanLimitConfigResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// Update updates loan limit config information
func (s *service) Update(ctx context.Context, id string, req UpdateLoanLimitConfigRequest) (*LoanLimitConfigResponse, error) {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanLimitConfigNotFound) {
			return nil, ErrLoanLimitConfigNotFound
		}
		slog.ErrorContext(ctx, "Update: failed to get loan limit config", slog.Any("error", err))
		return nil, err
	}

	// Update fields
	if req.RiskTier != nil {
		// Check for duplicate risk tier
		existing, err := s.repo.GetByRiskTier(ctx, *req.RiskTier)
		if err == nil && existing != nil && existing.ID != id {
			return nil, ErrRiskTierAlreadyExists
		}
		config.RiskTier = *req.RiskTier
	}
	if req.MinLoanAmount != nil {
		config.MinLoanAmount = *req.MinLoanAmount
	}
	if req.MaxLoanAmount != nil {
		config.MaxLoanAmount = *req.MaxLoanAmount
	}
	if req.IncomeMultiplierBps != nil {
		config.IncomeMultiplierBps = *req.IncomeMultiplierBps
	}
	if req.MaxConcurrentLoans != nil {
		config.MaxConcurrentLoans = *req.MaxConcurrentLoans
	}
	if req.MaxLoanDurationDays != nil {
		config.MaxLoanDurationDays = *req.MaxLoanDurationDays
	}
	if req.InterestRateBps != nil {
		config.InterestRateBps = *req.InterestRateBps
	}
	if req.LateFeeBps != nil {
		config.LateFeeBps = req.LateFeeBps
	}
	if req.DefaultPenaltyBps != nil {
		config.DefaultPenaltyBps = req.DefaultPenaltyBps
	}
	if req.IsActive != nil {
		config.IsActive = *req.IsActive
	}
	if req.UpdatedBy != nil {
		config.UpdatedBy = req.UpdatedBy
	}

	// Validate amount range
	if config.MaxLoanAmount <= config.MinLoanAmount {
		return nil, ErrInvalidAmountRange
	}

	// Update in database
	if err := s.repo.Update(ctx, config); err != nil {
		slog.ErrorContext(ctx, "Update: failed to update loan limit config", slog.Any("error", err))
		return nil, err
	}

	return toLoanLimitConfigResponse(config), nil
}

// Delete soft deletes a loan limit config
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrLoanLimitConfigNotFound) {
			return ErrLoanLimitConfigNotFound
		}
		slog.ErrorContext(ctx, "Delete: failed to delete loan limit config", slog.Any("error", err))
		return err
	}

	return nil
}

// Restore restores a soft-deleted loan limit config
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrLoanLimitConfigNotFound) {
			return ErrLoanLimitConfigNotFound
		}
		slog.ErrorContext(ctx, "Restore: failed to restore loan limit config", slog.Any("error", err))
		return err
	}

	return nil
}

// Activate activates a loan limit config with business validation
func (s *service) Activate(ctx context.Context, id string, activatedBy string) (*LoanLimitConfigResponse, error) {
	// Get the config
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanLimitConfigNotFound) {
			return nil, ErrLoanLimitConfigNotFound
		}
		slog.ErrorContext(ctx, "Activate: failed to get loan limit config", slog.Any("error", err))
		return nil, err
	}

	// Check if already active
	if config.IsActive {
		return nil, ErrConfigAlreadyActive
	}

	// Activate the config
	config.IsActive = true
	if activatedBy != "" {
		config.UpdatedBy = &activatedBy
	}

	// Update in database
	if err := s.repo.Update(ctx, config); err != nil {
		slog.ErrorContext(ctx, "Activate: failed to update loan limit config", slog.Any("error", err))
		return nil, err
	}

	slog.InfoContext(ctx, "Activate: successfully activated loan limit config", slog.String("id", id), slog.String("activated_by", activatedBy))
	return toLoanLimitConfigResponse(config), nil
}

// Deactivate deactivates a loan limit config with business validation
func (s *service) Deactivate(ctx context.Context, id string, deactivatedBy string) (*LoanLimitConfigResponse, error) {
	// Get the config
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrLoanLimitConfigNotFound) {
			return nil, ErrLoanLimitConfigNotFound
		}
		slog.ErrorContext(ctx, "Deactivate: failed to get loan limit config", slog.Any("error", err))
		return nil, err
	}

	// Check if already inactive
	if !config.IsActive {
		return nil, ErrConfigAlreadyInactive
	}

	// Deactivate the config
	config.IsActive = false
	if deactivatedBy != "" {
		config.UpdatedBy = &deactivatedBy
	}

	// Update in database
	if err := s.repo.Update(ctx, config); err != nil {
		slog.ErrorContext(ctx, "Deactivate: failed to update loan limit config", slog.Any("error", err))
		return nil, err
	}

	slog.InfoContext(ctx, "Deactivate: successfully deactivated loan limit config", slog.String("id", id), slog.String("deactivated_by", deactivatedBy))
	return toLoanLimitConfigResponse(config), nil
}

// --- Helper functions ---

// validateCreateRequest validates the create loan limit config request
func (s *service) validateCreateRequest(req CreateLoanLimitConfigRequest) error {
	if req.RiskTier == "" {
		return ErrInvalidRiskTier
	}

	if req.MinLoanAmount <= 0 {
		return ErrInvalidMinAmount
	}

	if req.MaxLoanAmount <= 0 {
		return ErrInvalidMaxAmount
	}

	if req.MaxLoanAmount <= req.MinLoanAmount {
		return ErrInvalidAmountRange
	}

	if req.InterestRateBps <= 0 {
		return ErrInvalidInterestRate
	}

	return nil
}

// toLoanLimitConfigResponse converts a loan limit config model to response DTO
func toLoanLimitConfigResponse(config *models.LoanLimitConfig) *LoanLimitConfigResponse {
	return &LoanLimitConfigResponse{
		ID:                  config.ID,
		RiskTier:            config.RiskTier,
		MinLoanAmount:       config.MinLoanAmount,
		MaxLoanAmount:       config.MaxLoanAmount,
		IncomeMultiplierBps: config.IncomeMultiplierBps,
		MaxConcurrentLoans:  config.MaxConcurrentLoans,
		MaxLoanDurationDays: config.MaxLoanDurationDays,
		InterestRateBps:     config.InterestRateBps,
		LateFeeBps:          config.LateFeeBps,
		DefaultPenaltyBps:   config.DefaultPenaltyBps,
		IsActive:            config.IsActive,
		CreatedAt:           config.CreatedAt,
		CreatedBy:           config.CreatedBy,
		UpdatedAt:           config.UpdatedAt,
		UpdatedBy:           config.UpdatedBy,
	}
}
