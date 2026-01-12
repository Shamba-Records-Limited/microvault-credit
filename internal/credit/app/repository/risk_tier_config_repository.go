package repository

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	pkgErrors "github.com/Shamba-Records-Limited/Microvault/pkg/errors"
	"gorm.io/gorm"
)

// Common errors for RiskTierConfigRepository
var (
	ErrRiskTierConfigNotFound             = errors.New("risk tier config not found")
	ErrFailedToCreateRiskTierConfig       = errors.New("failed to create risk tier config")
	ErrFailedToCreateBatchRiskTierConfigs = errors.New("failed to create batch risk tier configs")
	ErrFailedToGetRiskTierConfig          = errors.New("failed to get risk tier config")
	ErrFailedToGetRiskTierConfigByName    = errors.New("failed to get risk tier config by name")
	ErrFailedToGetRiskTierConfigByScore   = errors.New("failed to get risk tier config by score")
	ErrFailedToGetAllRiskTierConfigs      = errors.New("failed to get all risk tier configs")
	ErrFailedToGetActiveRiskTierConfigs   = errors.New("failed to get active risk tier configs")
	ErrFailedToUpdateRiskTierConfig       = errors.New("failed to update risk tier config")
	ErrFailedToRestoreRiskTierConfig      = errors.New("failed to restore risk tier config")
	ErrFailedToDeleteRiskTierConfig       = errors.New("failed to delete risk tier config")
)

// RiskTierConfigRepository defines the interface for risk tier config data access
type RiskTierConfigRepository interface {
	// Create operations
	Create(ctx context.Context, config *models.RiskTierConfig) error
	BatchCreate(ctx context.Context, configs []*models.RiskTierConfig) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.RiskTierConfig, error)
	GetByName(ctx context.Context, name string) (*models.RiskTierConfig, error)
	GetByScore(ctx context.Context, score int) (*models.RiskTierConfig, error)
	GetAll(ctx context.Context, limit, offset int) ([]*models.RiskTierConfig, error)
	GetActive(ctx context.Context, limit, offset int) ([]*models.RiskTierConfig, error)

	// Update operations
	Update(ctx context.Context, config *models.RiskTierConfig) error
	Restore(ctx context.Context, id string) error

	// Delete operations
	Delete(ctx context.Context, id string) error
}

// riskTierConfigRepository represents a repository for managing risk tier configs
type riskTierConfigRepository struct {
	db *gorm.DB
}

// NewRiskTierConfigRepository creates a new instance of RiskTierConfigRepository
func NewRiskTierConfigRepository(db *gorm.DB) (RiskTierConfigRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &riskTierConfigRepository{db: db}, nil
}

// --- Create Operations ---

// Create creates a new risk tier config
func (r *riskTierConfigRepository) Create(ctx context.Context, config *models.RiskTierConfig) error {
	result := r.db.WithContext(ctx).Create(config)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateRiskTierConfig
	}
	return nil
}

// BatchCreate creates multiple risk tier configs in a single batch
func (r *riskTierConfigRepository) BatchCreate(ctx context.Context, configs []*models.RiskTierConfig) error {
	if len(configs) == 0 {
		return nil
	}

	result := r.db.WithContext(ctx).Create(configs)
	if result.Error != nil {
		log.Printf("CreateBatch: database error: %v", result.Error)
		return ErrFailedToCreateBatchRiskTierConfigs
	}
	return nil
}

// --- Read Operations ---

// GetByID retrieves a risk tier config by its ID
func (r *riskTierConfigRepository) GetByID(ctx context.Context, id string) (*models.RiskTierConfig, error) {
	var config models.RiskTierConfig
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&config)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrRiskTierConfigNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetRiskTierConfig
	}
	return &config, nil
}

// GetByName retrieves a risk tier config by its name
func (r *riskTierConfigRepository) GetByName(ctx context.Context, name string) (*models.RiskTierConfig, error) {
	var config models.RiskTierConfig
	result := r.db.WithContext(ctx).
		Where("tier_name = ?", name).
		First(&config)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrRiskTierConfigNotFound
	}
	if result.Error != nil {
		log.Printf("GetByName: database error: %v", result.Error)
		return nil, ErrFailedToGetRiskTierConfigByName
	}
	return &config, nil
}

// GetByScore retrieves the risk tier config for a given credit score
func (r *riskTierConfigRepository) GetByScore(ctx context.Context, score int) (*models.RiskTierConfig, error) {
	var config models.RiskTierConfig
	result := r.db.WithContext(ctx).
		Where("min_score <= ? AND max_score >= ? AND is_active = ?", score, score, true).
		First(&config)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrRiskTierConfigNotFound
	}
	if result.Error != nil {
		log.Printf("GetByScore: database error: %v", result.Error)
		return nil, ErrFailedToGetRiskTierConfigByScore
	}
	return &config, nil
}

// GetAll retrieves all risk tier configs
func (r *riskTierConfigRepository) GetAll(ctx context.Context, limit, offset int) ([]*models.RiskTierConfig, error) {
	var configs []*models.RiskTierConfig
	result := r.db.WithContext(ctx).
		Order("tier_order ASC").
		Limit(limit).
		Offset(offset).
		Find(&configs)
	if result.Error != nil {
		log.Printf("GetAll: database error: %v", result.Error)
		return nil, ErrFailedToGetAllRiskTierConfigs
	}
	return configs, nil
}

// GetActive retrieves all active risk tier configs
func (r *riskTierConfigRepository) GetActive(ctx context.Context, limit, offset int) ([]*models.RiskTierConfig, error) {
	var configs []*models.RiskTierConfig
	result := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("tier_order ASC").
		Limit(limit).
		Offset(offset).
		Find(&configs)
	if result.Error != nil {
		log.Printf("GetActive: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveRiskTierConfigs
	}
	return configs, nil
}

// --- Update Operations ---

// Update updates a risk tier config
func (r *riskTierConfigRepository) Update(ctx context.Context, config *models.RiskTierConfig) error {
	result := r.db.WithContext(ctx).
		Model(config).
		Where("id = ?", config.ID).
		Updates(map[string]interface{}{
			"tier_name":   config.TierName,
			"min_score":   config.MinScore,
			"max_score":   config.MaxScore,
			"tier_order":  config.TierOrder,
			"description": config.Description,
			"color_code":  config.ColorCode,
			"is_active":   config.IsActive,
			"updated_at":  time.Now(),
		})
	if result.RowsAffected == 0 {
		return ErrRiskTierConfigNotFound
	}
	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateRiskTierConfig
	}
	return nil
}

// Restore restores a risk tier config by ID
func (r *riskTierConfigRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.RiskTierConfig{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrRiskTierConfigNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreRiskTierConfig
	}
	return nil
}

// --- Delete Operations ---

// Delete deletes a risk tier config by ID
func (r *riskTierConfigRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.RiskTierConfig{})
	if result.RowsAffected == 0 {
		return ErrRiskTierConfigNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteRiskTierConfig
	}
	return nil
}
