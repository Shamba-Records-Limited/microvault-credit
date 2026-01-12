package risktierconfig

import (
	"context"
	"errors"
	"log"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
)

// Service defines the interface for risk tier config business logic operations
type Service interface {
	// Risk tier config management
	Create(ctx context.Context, req CreateRiskTierConfigRequest) (*RiskTierConfigResponse, error)
	BatchCreate(ctx context.Context, reqs []CreateRiskTierConfigRequest) ([]*RiskTierConfigResponse, error)
	GetByID(ctx context.Context, id string) (*RiskTierConfigResponse, error)
	GetByName(ctx context.Context, name string) (*RiskTierConfigResponse, error)
	GetByScore(ctx context.Context, score int) (*RiskTierConfigResponse, error)
	GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[RiskTierConfigResponse], error)
	GetActive(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[RiskTierConfigResponse], error)
	Update(ctx context.Context, id string, req UpdateRiskTierConfigRequest) (*RiskTierConfigResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error

	// State management
	Activate(ctx context.Context, id string, activatedBy string) (*RiskTierConfigResponse, error)
	Deactivate(ctx context.Context, id string, deactivatedBy string) (*RiskTierConfigResponse, error)
}

// service implements the Service interface
type service struct {
	repo repository.RiskTierConfigRepository
}

// NewService creates a new risk tier config service instance
func NewService(repo repository.RiskTierConfigRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new risk tier config with business validation
func (s *service) Create(ctx context.Context, req CreateRiskTierConfigRequest) (*RiskTierConfigResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Check for duplicate tier name
	existing, err := s.repo.GetByName(ctx, req.TierName)
	if err == nil && existing != nil {
		return nil, ErrTierNameAlreadyExists
	}
	if err != nil && !errors.Is(err, repository.ErrRiskTierConfigNotFound) {
		log.Printf("Create: failed to check duplicate tier name: %v", err)
		return nil, err
	}

	// Create risk tier config model
	config := &models.RiskTierConfig{
		TierName:    req.TierName,
		MinScore:    req.MinScore,
		MaxScore:    req.MaxScore,
		TierOrder:   req.TierOrder,
		Description: req.Description,
		ColorCode:   req.ColorCode,
		IsActive:    req.IsActive,
		CreatedBy:   req.CreatedBy,
	}

	// Create config in database
	if err := s.repo.Create(ctx, config); err != nil {
		log.Printf("Create: failed to create risk tier config: %v", err)
		return nil, err
	}

	return toRiskTierConfigResponse(config), nil
}

// BatchCreate creates multiple risk tier configs
func (s *service) BatchCreate(ctx context.Context, reqs []CreateRiskTierConfigRequest) ([]*RiskTierConfigResponse, error) {
	if len(reqs) == 0 {
		return []*RiskTierConfigResponse{}, nil
	}

	configs := make([]*models.RiskTierConfig, len(reqs))
	for i, req := range reqs {
		if err := s.validateCreateRequest(req); err != nil {
			return nil, err
		}

		configs[i] = &models.RiskTierConfig{
			TierName:    req.TierName,
			MinScore:    req.MinScore,
			MaxScore:    req.MaxScore,
			TierOrder:   req.TierOrder,
			Description: req.Description,
			ColorCode:   req.ColorCode,
			IsActive:    req.IsActive,
		}
	}

	if err := s.repo.BatchCreate(ctx, configs); err != nil {
		log.Printf("BatchCreate: failed to create configs: %v", err)
		return nil, err
	}

	responses := make([]*RiskTierConfigResponse, len(configs))
	for i, config := range configs {
		responses[i] = toRiskTierConfigResponse(config)
	}

	return responses, nil
}

// GetByID retrieves a risk tier config by ID
func (s *service) GetByID(ctx context.Context, id string) (*RiskTierConfigResponse, error) {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrRiskTierConfigNotFound) {
			return nil, ErrRiskTierConfigNotFound
		}
		log.Printf("GetByID: failed to get risk tier config: %v", err)
		return nil, err
	}

	return toRiskTierConfigResponse(config), nil
}

// GetByName retrieves a risk tier config by name
func (s *service) GetByName(ctx context.Context, name string) (*RiskTierConfigResponse, error) {
	config, err := s.repo.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, repository.ErrRiskTierConfigNotFound) {
			return nil, ErrRiskTierConfigNotFound
		}
		log.Printf("GetByName: failed to get risk tier config: %v", err)
		return nil, err
	}

	return toRiskTierConfigResponse(config), nil
}

// GetByScore retrieves the risk tier config for a given credit score
func (s *service) GetByScore(ctx context.Context, score int) (*RiskTierConfigResponse, error) {
	config, err := s.repo.GetByScore(ctx, score)
	if err != nil {
		if errors.Is(err, repository.ErrRiskTierConfigNotFound) {
			return nil, ErrRiskTierConfigNotFound
		}
		log.Printf("GetByScore: failed to get risk tier config: %v", err)
		return nil, err
	}

	return toRiskTierConfigResponse(config), nil
}

// GetAll retrieves all risk tier configs with pagination
func (s *service) GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[RiskTierConfigResponse], error) {
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
		log.Printf("GetAll: failed to get risk tier configs: %v", err)
		return nil, err
	}

	responses := make([]RiskTierConfigResponse, len(configs))
	for i, config := range configs {
		responses[i] = *toRiskTierConfigResponse(config)
	}

	return &services.PaginatedResponse[RiskTierConfigResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetActive retrieves active risk tier configs with pagination
func (s *service) GetActive(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[RiskTierConfigResponse], error) {
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
		log.Printf("GetActive: failed to get active risk tier configs: %v", err)
		return nil, err
	}

	responses := make([]RiskTierConfigResponse, len(configs))
	for i, config := range configs {
		responses[i] = *toRiskTierConfigResponse(config)
	}

	return &services.PaginatedResponse[RiskTierConfigResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// Update updates risk tier config information
func (s *service) Update(ctx context.Context, id string, req UpdateRiskTierConfigRequest) (*RiskTierConfigResponse, error) {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrRiskTierConfigNotFound) {
			return nil, ErrRiskTierConfigNotFound
		}
		log.Printf("Update: failed to get risk tier config: %v", err)
		return nil, err
	}

	// Update fields
	if req.TierName != nil {
		// Check for duplicate tier name
		existing, err := s.repo.GetByName(ctx, *req.TierName)
		if err == nil && existing != nil && existing.ID != id {
			return nil, ErrTierNameAlreadyExists
		}
		config.TierName = *req.TierName
	}
	if req.MinScore != nil {
		config.MinScore = *req.MinScore
	}
	if req.MaxScore != nil {
		config.MaxScore = *req.MaxScore
	}
	if req.TierOrder != nil {
		config.TierOrder = *req.TierOrder
	}
	if req.Description != nil {
		config.Description = req.Description
	}
	if req.ColorCode != nil {
		config.ColorCode = req.ColorCode
	}
	if req.IsActive != nil {
		config.IsActive = *req.IsActive
	}

	// Validate score range
	if config.MaxScore <= config.MinScore {
		return nil, ErrInvalidScoreRange
	}

	// Update in database
	if err := s.repo.Update(ctx, config); err != nil {
		log.Printf("Update: failed to update risk tier config: %v", err)
		return nil, err
	}

	return toRiskTierConfigResponse(config), nil
}

// Delete soft deletes a risk tier config
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrRiskTierConfigNotFound) {
			return ErrRiskTierConfigNotFound
		}
		log.Printf("Delete: failed to delete risk tier config: %v", err)
		return err
	}

	return nil
}

// Restore restores a soft-deleted risk tier config
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrRiskTierConfigNotFound) {
			return ErrRiskTierConfigNotFound
		}
		log.Printf("Restore: failed to restore risk tier config: %v", err)
		return err
	}

	return nil
}

// Activate activates a risk tier config with business validation
func (s *service) Activate(ctx context.Context, id string, activatedBy string) (*RiskTierConfigResponse, error) {
	// Get the config
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrRiskTierConfigNotFound) {
			return nil, ErrRiskTierConfigNotFound
		}
		log.Printf("Activate: failed to get risk tier config: %v", err)
		return nil, err
	}

	// Check if already active
	if config.IsActive {
		return nil, ErrTierAlreadyActive
	}

	// Activate the config
	config.IsActive = true
	if activatedBy != "" {
		config.UpdatedBy = &activatedBy
	}

	// Update in database
	if err := s.repo.Update(ctx, config); err != nil {
		log.Printf("Activate: failed to update risk tier config: %v", err)
		return nil, err
	}

	log.Printf("Activate: successfully activated risk tier config %s by %s", id, activatedBy)
	return toRiskTierConfigResponse(config), nil
}

// Deactivate deactivates a risk tier config with business validation
func (s *service) Deactivate(ctx context.Context, id string, deactivatedBy string) (*RiskTierConfigResponse, error) {
	// Get the config
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrRiskTierConfigNotFound) {
			return nil, ErrRiskTierConfigNotFound
		}
		log.Printf("Deactivate: failed to get risk tier config: %v", err)
		return nil, err
	}

	// Check if already inactive
	if !config.IsActive {
		return nil, ErrTierAlreadyInactive
	}

	// Deactivate the config
	config.IsActive = false
	if deactivatedBy != "" {
		config.UpdatedBy = &deactivatedBy
	}

	// Update in database
	if err := s.repo.Update(ctx, config); err != nil {
		log.Printf("Deactivate: failed to update risk tier config: %v", err)
		return nil, err
	}

	log.Printf("Deactivate: successfully deactivated risk tier config %s by %s", id, deactivatedBy)
	return toRiskTierConfigResponse(config), nil
}

// --- Helper functions ---

// validateCreateRequest validates the create risk tier config request
func (s *service) validateCreateRequest(req CreateRiskTierConfigRequest) error {
	if req.TierName == "" {
		return ErrInvalidTierName
	}

	if req.MinScore < 0 {
		return ErrInvalidMinScore
	}

	if req.MaxScore <= 0 {
		return ErrInvalidMaxScore
	}

	if req.MaxScore <= req.MinScore {
		return ErrInvalidScoreRange
	}

	if req.TierOrder < 0 {
		return ErrInvalidTierOrder
	}

	return nil
}

// toRiskTierConfigResponse converts a risk tier config model to response DTO
func toRiskTierConfigResponse(config *models.RiskTierConfig) *RiskTierConfigResponse {
	return &RiskTierConfigResponse{
		ID:          config.ID,
		TierName:    config.TierName,
		MinScore:    config.MinScore,
		MaxScore:    config.MaxScore,
		TierOrder:   config.TierOrder,
		Description: config.Description,
		ColorCode:   config.ColorCode,
		IsActive:    config.IsActive,
		CreatedAt:   config.CreatedAt,
		UpdatedAt:   config.UpdatedAt,
		CreatedBy:   config.CreatedBy,
		UpdatedBy:   config.UpdatedBy,
	}
}
