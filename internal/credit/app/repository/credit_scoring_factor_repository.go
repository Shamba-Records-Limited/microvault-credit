package repository

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	"gorm.io/gorm"
)

// Common errors for CreditScoringFactorRepository
var (
	ErrCreditScoringFactorNotFound             = errors.New("credit scoring factor not found")
	ErrFailedToCreateCreditScoringFactor       = errors.New("failed to create credit scoring factor")
	ErrFailedToCreateBatchCreditScoringFactors = errors.New("failed to create batch credit scoring factors")
	ErrFailedToGetCreditScoringFactor          = errors.New("failed to get credit scoring factor")
	ErrFailedToGetCreditScoringFactorByName    = errors.New("failed to get credit scoring factor by name")
	ErrFailedToGetAllCreditScoringFactors      = errors.New("failed to get all credit scoring factors")
	ErrFailedToGetActiveCreditScoringFactors   = errors.New("failed to get active credit scoring factors")
	ErrFailedToUpdateCreditScoringFactor       = errors.New("failed to update credit scoring factor")
	ErrFailedToRestoreCreditScoringFactor      = errors.New("failed to restore credit scoring factor")
	ErrFailedToDeleteCreditScoringFactor       = errors.New("failed to delete credit scoring factor")
)

// CreditScoringFactorRepository defines the interface for credit scoring factor data access
type CreditScoringFactorRepository interface {
	// Create operations
	Create(ctx context.Context, factor *models.CreditScoringFactor) error
	BatchCreate(ctx context.Context, factors []*models.CreditScoringFactor) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.CreditScoringFactor, error)
	GetByName(ctx context.Context, name string) (*models.CreditScoringFactor, error)
	GetAll(ctx context.Context, limit, offset int) ([]*models.CreditScoringFactor, error)
	GetActive(ctx context.Context, limit, offset int) ([]*models.CreditScoringFactor, error)

	// Update operations
	Update(ctx context.Context, factor *models.CreditScoringFactor) error
	Restore(ctx context.Context, id string) error

	// Delete operations
	Delete(ctx context.Context, id string) error
}

// creditScoringFactorRepository represents a repository for managing credit scoring factors
type creditScoringFactorRepository struct {
	db *gorm.DB
}

// NewCreditScoringFactorRepository creates a new instance of CreditScoringFactorRepository
func NewCreditScoringFactorRepository(db *gorm.DB) (CreditScoringFactorRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &creditScoringFactorRepository{db: db}, nil
}

// --- Create Operations ---

// Create creates a new credit scoring factor
func (r *creditScoringFactorRepository) Create(ctx context.Context, factor *models.CreditScoringFactor) error {
	result := r.db.WithContext(ctx).Create(factor)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateCreditScoringFactor
	}
	return nil
}

// BatchCreate creates multiple credit scoring factors in a single batch
func (r *creditScoringFactorRepository) BatchCreate(ctx context.Context, factors []*models.CreditScoringFactor) error {
	if len(factors) == 0 {
		return nil
	}

	result := r.db.WithContext(ctx).Create(factors)
	if result.Error != nil {
		log.Printf("CreateBatch: database error: %v", result.Error)
		return ErrFailedToCreateBatchCreditScoringFactors
	}
	return nil
}

// --- Read Operations ---

// GetByID retrieves a credit scoring factor by its ID
func (r *creditScoringFactorRepository) GetByID(ctx context.Context, id string) (*models.CreditScoringFactor, error) {
	var factor models.CreditScoringFactor
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&factor)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrCreditScoringFactorNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditScoringFactor
	}
	return &factor, nil
}

// GetByName retrieves a credit scoring factor by its name
func (r *creditScoringFactorRepository) GetByName(ctx context.Context, name string) (*models.CreditScoringFactor, error) {
	var factor models.CreditScoringFactor
	result := r.db.WithContext(ctx).
		Where("factor_name = ?", name).
		First(&factor)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrCreditScoringFactorNotFound
	}
	if result.Error != nil {
		log.Printf("GetByName: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditScoringFactorByName
	}
	return &factor, nil
}

// GetAll retrieves all credit scoring factors
func (r *creditScoringFactorRepository) GetAll(ctx context.Context, limit, offset int) ([]*models.CreditScoringFactor, error) {
	var factors []*models.CreditScoringFactor
	result := r.db.WithContext(ctx).
		Order("factor_name ASC").
		Limit(limit).
		Offset(offset).
		Find(&factors)
	if result.Error != nil {
		log.Printf("GetAll: database error: %v", result.Error)
		return nil, ErrFailedToGetAllCreditScoringFactors
	}
	return factors, nil
}

// GetActive retrieves all active credit scoring factors
func (r *creditScoringFactorRepository) GetActive(ctx context.Context, limit, offset int) ([]*models.CreditScoringFactor, error) {
	var factors []*models.CreditScoringFactor
	result := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("factor_name ASC").
		Limit(limit).
		Offset(offset).
		Find(&factors)
	if result.Error != nil {
		log.Printf("GetActive: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveCreditScoringFactors
	}
	return factors, nil
}

// --- Update Operations ---

// Update updates a credit scoring factor
func (r *creditScoringFactorRepository) Update(ctx context.Context, factor *models.CreditScoringFactor) error {
	result := r.db.WithContext(ctx).
		Model(factor).
		Where("id = ?", factor.ID).
		Updates(map[string]interface{}{
			"factor_name":        factor.FactorName,
			"factor_description": factor.FactorDescription,
			"weight":             factor.WeightBps,
			"is_active":          factor.IsActive,
			"calculation_method": factor.CalculationMethod,
			"min_data_required":  factor.MinDataRequired,
			"updated_by":         factor.UpdatedBy,
			"updated_at":         time.Now(),
		})
	if result.RowsAffected == 0 {
		return ErrCreditScoringFactorNotFound
	}
	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateCreditScoringFactor
	}
	return nil
}

// Restore restores a credit scoring factor by ID
func (r *creditScoringFactorRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.CreditScoringFactor{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrCreditScoringFactorNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreCreditScoringFactor
	}
	return nil
}

// --- Delete Operations ---

// Delete deletes a credit scoring factor by ID
func (r *creditScoringFactorRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.CreditScoringFactor{})
	if result.RowsAffected == 0 {
		return ErrCreditScoringFactorNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteCreditScoringFactor
	}
	return nil
}
