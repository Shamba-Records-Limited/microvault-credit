package repository

import (
	"context"
	"errors"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
)

// Common errors for GlobalLendingLimitRepository
var (
	ErrGlobalLendingLimitNotFound               = errors.New("global lending limit not found")
	ErrFailedToCreateGlobalLendingLimit         = errors.New("failed to create global lending limit")
	ErrFailedToCreateBatchGlobalLendingLimits   = errors.New("failed to create batch global lending limits")
	ErrFailedToGetGlobalLendingLimit            = errors.New("failed to get global lending limit")
	ErrFailedToGetGlobalLendingLimitByKey       = errors.New("failed to get global lending limit by key")
	ErrFailedToGetGlobalLendingLimitsByCategory = errors.New("failed to get global lending limits by category")
	ErrFailedToGetAllGlobalLendingLimits        = errors.New("failed to get all global lending limits")
	ErrFailedToGetActiveGlobalLendingLimits     = errors.New("failed to get active global lending limits")
	ErrFailedToUpdateGlobalLendingLimit         = errors.New("failed to update global lending limit")
	ErrFailedToRestoreGlobalLendingLimit        = errors.New("failed to restore global lending limit")
	ErrFailedToDeleteGlobalLendingLimit         = errors.New("failed to delete global lending limit")
)

// GlobalLendingLimitRepository defines the interface for global lending limit data access
type GlobalLendingLimitRepository interface {
	// Create operations
	Create(ctx context.Context, limit *models.GlobalLendingLimit) error
	BatchCreate(ctx context.Context, limits []*models.GlobalLendingLimit) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.GlobalLendingLimit, error)
	GetByKey(ctx context.Context, key string) (*models.GlobalLendingLimit, error)
	GetByCategory(ctx context.Context, category string, limit, offset int) ([]*models.GlobalLendingLimit, error)
	GetAll(ctx context.Context, limit, offset int) ([]*models.GlobalLendingLimit, error)
	GetActive(ctx context.Context, limit, offset int) ([]*models.GlobalLendingLimit, error)

	// Update operations
	Update(ctx context.Context, limit *models.GlobalLendingLimit) error
	Restore(ctx context.Context, id string) error

	// Delete operations
	Delete(ctx context.Context, id string) error
}

// globalLendingLimitRepository represents a repository for managing global lending limits
type globalLendingLimitRepository struct {
	db *gorm.DB
}

// NewGlobalLendingLimitRepository creates a new instance of GlobalLendingLimitRepository
func NewGlobalLendingLimitRepository(db *gorm.DB) (GlobalLendingLimitRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &globalLendingLimitRepository{db: db}, nil
}

// --- Create Operations ---

// Create creates a new global lending limit
func (r *globalLendingLimitRepository) Create(ctx context.Context, limit *models.GlobalLendingLimit) error {
	result := r.db.WithContext(ctx).Create(limit)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateGlobalLendingLimit
	}
	return nil
}

// BatchCreate creates multiple global lending limits in a single batch
func (r *globalLendingLimitRepository) BatchCreate(ctx context.Context, limits []*models.GlobalLendingLimit) error {
	if len(limits) == 0 {
		return nil
	}

	result := r.db.WithContext(ctx).Create(limits)
	if result.Error != nil {
		log.Printf("CreateBatch: database error: %v", result.Error)
		return ErrFailedToCreateBatchGlobalLendingLimits
	}
	return nil
}

// --- Read Operations ---

// GetByID retrieves a global lending limit by its ID
func (r *globalLendingLimitRepository) GetByID(ctx context.Context, id string) (*models.GlobalLendingLimit, error) {
	var limit models.GlobalLendingLimit
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&limit)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrGlobalLendingLimitNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetGlobalLendingLimit
	}
	return &limit, nil
}

// GetByKey retrieves a global lending limit by its config key
func (r *globalLendingLimitRepository) GetByKey(ctx context.Context, key string) (*models.GlobalLendingLimit, error) {
	var limit models.GlobalLendingLimit
	result := r.db.WithContext(ctx).
		Where("config_key = ? AND is_active = ?", key, true).
		First(&limit)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrGlobalLendingLimitNotFound
	}
	if result.Error != nil {
		log.Printf("GetByKey: database error: %v", result.Error)
		return nil, ErrFailedToGetGlobalLendingLimitByKey
	}
	return &limit, nil
}

// GetByCategory retrieves global lending limits by category
func (r *globalLendingLimitRepository) GetByCategory(ctx context.Context, category string, limit, offset int) ([]*models.GlobalLendingLimit, error) {
	var limits []*models.GlobalLendingLimit
	result := r.db.WithContext(ctx).
		Where("category = ?", category).
		Order("config_key ASC").
		Limit(limit).
		Offset(offset).
		Find(&limits)
	if result.Error != nil {
		log.Printf("GetByCategory: database error: %v", result.Error)
		return nil, ErrFailedToGetGlobalLendingLimitsByCategory
	}
	return limits, nil
}

// GetAll retrieves all global lending limits
func (r *globalLendingLimitRepository) GetAll(ctx context.Context, limit, offset int) ([]*models.GlobalLendingLimit, error) {
	var limits []*models.GlobalLendingLimit
	result := r.db.WithContext(ctx).
		Order("category ASC, config_key ASC").
		Limit(limit).
		Offset(offset).
		Find(&limits)
	if result.Error != nil {
		log.Printf("GetAll: database error: %v", result.Error)
		return nil, ErrFailedToGetAllGlobalLendingLimits
	}
	return limits, nil
}

// GetActive retrieves all active global lending limits
func (r *globalLendingLimitRepository) GetActive(ctx context.Context, limit, offset int) ([]*models.GlobalLendingLimit, error) {
	var limits []*models.GlobalLendingLimit
	result := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("category ASC, config_key ASC").
		Limit(limit).
		Offset(offset).
		Find(&limits)
	if result.Error != nil {
		log.Printf("GetActive: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveGlobalLendingLimits
	}
	return limits, nil
}

// --- Update Operations ---

// Update updates a global lending limit
func (r *globalLendingLimitRepository) Update(ctx context.Context, limit *models.GlobalLendingLimit) error {
	result := r.db.WithContext(ctx).
		Model(limit).
		Where("id = ?", limit.ID).
		Updates(map[string]interface{}{
			"config_key":   limit.ConfigKey,
			"config_value": limit.ConfigValue,
			"value_type":   limit.ValueType,
			"description":  limit.Description,
			"category":     limit.Category,
			"is_active":    limit.IsActive,
			"updated_by":   limit.UpdatedBy,
			"updated_at":   time.Now(),
		})
	if result.RowsAffected == 0 {
		return ErrGlobalLendingLimitNotFound
	}
	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateGlobalLendingLimit
	}
	return nil
}

// Restore restores a global lending limit by ID
func (r *globalLendingLimitRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.GlobalLendingLimit{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrGlobalLendingLimitNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreGlobalLendingLimit
	}
	return nil
}

// --- Delete Operations ---

// Delete deletes a global lending limit by ID
func (r *globalLendingLimitRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.GlobalLendingLimit{})
	if result.RowsAffected == 0 {
		return ErrGlobalLendingLimitNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteGlobalLendingLimit
	}
	return nil
}
