package globallendinglimit

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
)

// Valid value types
var validValueTypes = map[string]bool{
	models.ConfigValueTypeInteger: true,
	models.ConfigValueTypeDecimal: true,
	models.ConfigValueTypeBoolean: true,
	models.ConfigValueTypeString:  true,
}

// Service defines the interface for global lending limit business logic operations
type Service interface {
	// Global lending limit management
	Create(ctx context.Context, req CreateGlobalLendingLimitRequest) (*GlobalLendingLimitResponse, error)
	BatchCreate(ctx context.Context, reqs []CreateGlobalLendingLimitRequest) ([]*GlobalLendingLimitResponse, error)
	GetByID(ctx context.Context, id string) (*GlobalLendingLimitResponse, error)
	GetByKey(ctx context.Context, key string) (*GlobalLendingLimitResponse, error)
	GetByCategory(ctx context.Context, category string, pagination services.Pagination) (*services.PaginatedResponse[GlobalLendingLimitResponse], error)
	GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[GlobalLendingLimitResponse], error)
	GetActive(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[GlobalLendingLimitResponse], error)
	Update(ctx context.Context, id string, req UpdateGlobalLendingLimitRequest) (*GlobalLendingLimitResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error

	// State management
	Activate(ctx context.Context, id string, activatedBy string) (*GlobalLendingLimitResponse, error)
	Deactivate(ctx context.Context, id string, deactivatedBy string) (*GlobalLendingLimitResponse, error)
}

// service implements the Service interface
type service struct {
	repo repository.GlobalLendingLimitRepository
}

// NewService creates a new global lending limit service instance
func NewService(repo repository.GlobalLendingLimitRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new global lending limit with business validation
func (s *service) Create(ctx context.Context, req CreateGlobalLendingLimitRequest) (*GlobalLendingLimitResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Check for duplicate key
	existing, err := s.repo.GetByKey(ctx, req.ConfigKey)
	if err == nil && existing != nil {
		return nil, ErrConfigKeyAlreadyExists
	}
	if err != nil && !errors.Is(err, repository.ErrGlobalLendingLimitNotFound) {
		slog.ErrorContext(ctx, "Create: failed to check duplicate key", slog.Any("error", err))
		return nil, err
	}

	// Create global lending limit model
	limit := &models.GlobalLendingLimit{
		ConfigKey:   req.ConfigKey,
		ConfigValue: req.ConfigValue,
		ValueType:   req.ValueType,
		Description: req.Description,
		Category:    req.Category,
		IsActive:    req.IsActive,
		CreatedBy:   req.CreatedBy,
	}

	// Create limit in database
	if err := s.repo.Create(ctx, limit); err != nil {
		slog.ErrorContext(ctx, "Create: failed to create global lending limit", slog.Any("error", err))
		return nil, err
	}

	return toGlobalLendingLimitResponse(limit), nil
}

// BatchCreate creates multiple global lending limits
func (s *service) BatchCreate(ctx context.Context, reqs []CreateGlobalLendingLimitRequest) ([]*GlobalLendingLimitResponse, error) {
	if len(reqs) == 0 {
		return []*GlobalLendingLimitResponse{}, nil
	}

	limits := make([]*models.GlobalLendingLimit, len(reqs))
	for i, req := range reqs {
		if err := s.validateCreateRequest(req); err != nil {
			return nil, err
		}

		limits[i] = &models.GlobalLendingLimit{
			ConfigKey:   req.ConfigKey,
			ConfigValue: req.ConfigValue,
			ValueType:   req.ValueType,
			Description: req.Description,
			Category:    req.Category,
			IsActive:    req.IsActive,
			CreatedBy:   req.CreatedBy,
		}
	}

	if err := s.repo.BatchCreate(ctx, limits); err != nil {
		slog.ErrorContext(ctx, "BatchCreate: failed to create limits", slog.Any("error", err))
		return nil, err
	}

	responses := make([]*GlobalLendingLimitResponse, len(limits))
	for i, limit := range limits {
		responses[i] = toGlobalLendingLimitResponse(limit)
	}

	return responses, nil
}

// GetByID retrieves a global lending limit by ID
func (s *service) GetByID(ctx context.Context, id string) (*GlobalLendingLimitResponse, error) {
	limit, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrGlobalLendingLimitNotFound) {
			return nil, ErrGlobalLendingLimitNotFound
		}
		slog.ErrorContext(ctx, "GetByID: failed to get global lending limit", slog.Any("error", err))
		return nil, err
	}

	return toGlobalLendingLimitResponse(limit), nil
}

// GetByKey retrieves a global lending limit by config key
func (s *service) GetByKey(ctx context.Context, key string) (*GlobalLendingLimitResponse, error) {
	limit, err := s.repo.GetByKey(ctx, key)
	if err != nil {
		if errors.Is(err, repository.ErrGlobalLendingLimitNotFound) {
			return nil, ErrGlobalLendingLimitNotFound
		}
		slog.ErrorContext(ctx, "GetByKey: failed to get global lending limit", slog.Any("error", err))
		return nil, err
	}

	return toGlobalLendingLimitResponse(limit), nil
}

// GetByCategory retrieves global lending limits by category with pagination
func (s *service) GetByCategory(ctx context.Context, category string, pagination services.Pagination) (*services.PaginatedResponse[GlobalLendingLimitResponse], error) {
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

	limits, err := s.repo.GetByCategory(ctx, category, pagination.PageSize, offset)
	if err != nil {
		slog.ErrorContext(ctx, "GetByCategory: failed to get global lending limits", slog.Any("error", err))
		return nil, err
	}

	responses := make([]GlobalLendingLimitResponse, len(limits))
	for i, limit := range limits {
		responses[i] = *toGlobalLendingLimitResponse(limit)
	}

	return &services.PaginatedResponse[GlobalLendingLimitResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetAll retrieves all global lending limits with pagination
func (s *service) GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[GlobalLendingLimitResponse], error) {
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

	limits, err := s.repo.GetAll(ctx, pagination.PageSize, offset)
	if err != nil {
		slog.ErrorContext(ctx, "GetAll: failed to get global lending limits", slog.Any("error", err))
		return nil, err
	}

	responses := make([]GlobalLendingLimitResponse, len(limits))
	for i, limit := range limits {
		responses[i] = *toGlobalLendingLimitResponse(limit)
	}

	return &services.PaginatedResponse[GlobalLendingLimitResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetActive retrieves active global lending limits with pagination
func (s *service) GetActive(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[GlobalLendingLimitResponse], error) {
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

	limits, err := s.repo.GetActive(ctx, pagination.PageSize, offset)
	if err != nil {
		slog.ErrorContext(ctx, "GetActive: failed to get active global lending limits", slog.Any("error", err))
		return nil, err
	}

	responses := make([]GlobalLendingLimitResponse, len(limits))
	for i, limit := range limits {
		responses[i] = *toGlobalLendingLimitResponse(limit)
	}

	return &services.PaginatedResponse[GlobalLendingLimitResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// Update updates global lending limit information
func (s *service) Update(ctx context.Context, id string, req UpdateGlobalLendingLimitRequest) (*GlobalLendingLimitResponse, error) {
	limit, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrGlobalLendingLimitNotFound) {
			return nil, ErrGlobalLendingLimitNotFound
		}
		slog.ErrorContext(ctx, "Update: failed to get global lending limit", slog.Any("error", err))
		return nil, err
	}

	// Update fields
	if req.ConfigKey != nil {
		// Check for duplicate key
		existing, err := s.repo.GetByKey(ctx, *req.ConfigKey)
		if err == nil && existing != nil && existing.ID != id {
			return nil, ErrConfigKeyAlreadyExists
		}
		limit.ConfigKey = *req.ConfigKey
	}
	if req.ConfigValue != nil {
		limit.ConfigValue = *req.ConfigValue
	}
	if req.ValueType != nil {
		if !validValueTypes[*req.ValueType] {
			return nil, ErrInvalidValueType
		}
		limit.ValueType = *req.ValueType
	}
	if req.Description != nil {
		limit.Description = req.Description
	}
	if req.Category != nil {
		limit.Category = req.Category
	}
	if req.IsActive != nil {
		limit.IsActive = *req.IsActive
	}
	if req.UpdatedBy != nil {
		limit.UpdatedBy = req.UpdatedBy
	}

	// Update in database
	if err := s.repo.Update(ctx, limit); err != nil {
		slog.ErrorContext(ctx, "Update: failed to update global lending limit", slog.Any("error", err))
		return nil, err
	}

	return toGlobalLendingLimitResponse(limit), nil
}

// Delete soft deletes a global lending limit
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrGlobalLendingLimitNotFound) {
			return ErrGlobalLendingLimitNotFound
		}
		slog.ErrorContext(ctx, "Delete: failed to delete global lending limit", slog.Any("error", err))
		return err
	}

	return nil
}

// Restore restores a soft-deleted global lending limit
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrGlobalLendingLimitNotFound) {
			return ErrGlobalLendingLimitNotFound
		}
		slog.ErrorContext(ctx, "Restore: failed to restore global lending limit", slog.Any("error", err))
		return err
	}

	return nil
}

// Activate activates a global lending limit config with business validation
func (s *service) Activate(ctx context.Context, id string, activatedBy string) (*GlobalLendingLimitResponse, error) {
	// Get the config
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrGlobalLendingLimitNotFound) {
			return nil, ErrGlobalLendingLimitNotFound
		}
		slog.ErrorContext(ctx, "Activate: failed to get global lending limit", slog.Any("error", err))
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
		slog.ErrorContext(ctx, "Activate: failed to update global lending limit", slog.Any("error", err))
		return nil, err
	}

	slog.InfoContext(ctx, "Activate: successfully activated global lending limit", slog.String("id", id), slog.String("activated_by", activatedBy))
	return toGlobalLendingLimitResponse(config), nil
}

// Deactivate deactivates a global lending limit config with business validation
func (s *service) Deactivate(ctx context.Context, id string, deactivatedBy string) (*GlobalLendingLimitResponse, error) {
	// Get the config
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrGlobalLendingLimitNotFound) {
			return nil, ErrGlobalLendingLimitNotFound
		}
		slog.ErrorContext(ctx, "Deactivate: failed to get global lending limit", slog.Any("error", err))
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
		slog.ErrorContext(ctx, "Deactivate: failed to update global lending limit", slog.Any("error", err))
		return nil, err
	}

	slog.InfoContext(ctx, "Deactivate: successfully deactivated global lending limit", slog.String("id", id), slog.String("deactivated_by", deactivatedBy))
	return toGlobalLendingLimitResponse(config), nil
}

// --- Helper functions ---

// validateCreateRequest validates the create global lending limit request
func (s *service) validateCreateRequest(req CreateGlobalLendingLimitRequest) error {
	if req.ConfigKey == "" {
		return ErrInvalidConfigKey
	}

	if req.ConfigValue == "" {
		return ErrInvalidConfigValue
	}

	if !validValueTypes[req.ValueType] {
		return ErrInvalidValueType
	}

	return nil
}

// toGlobalLendingLimitResponse converts a global lending limit model to response DTO
func toGlobalLendingLimitResponse(limit *models.GlobalLendingLimit) *GlobalLendingLimitResponse {
	return &GlobalLendingLimitResponse{
		ID:          limit.ID,
		ConfigKey:   limit.ConfigKey,
		ConfigValue: limit.ConfigValue,
		ValueType:   limit.ValueType,
		Description: limit.Description,
		Category:    limit.Category,
		IsActive:    limit.IsActive,
		CreatedAt:   limit.CreatedAt,
		CreatedBy:   limit.CreatedBy,
		UpdatedAt:   limit.UpdatedAt,
		UpdatedBy:   limit.UpdatedBy,
	}
}
