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

// Common errors for LoanLimitConfigRepository
var (
	ErrLoanLimitConfigNotFound             = errors.New("loan limit config not found")
	ErrFailedToCreateLoanLimitConfig       = errors.New("failed to create loan limit config")
	ErrFailedToCreateBatchLoanLimitConfigs = errors.New("failed to create batch loan limit configs")
	ErrFailedToGetLoanLimitConfig          = errors.New("failed to get loan limit config")
	ErrFailedToGetLoanLimitConfigByTier    = errors.New("failed to get loan limit config by tier")
	ErrFailedToGetAllLoanLimitConfigs      = errors.New("failed to get all loan limit configs")
	ErrFailedToGetActiveLoanLimitConfigs   = errors.New("failed to get active loan limit configs")
	ErrFailedToUpdateLoanLimitConfig       = errors.New("failed to update loan limit config")
	ErrFailedToRestoreLoanLimitConfig      = errors.New("failed to restore loan limit config")
	ErrFailedToDeleteLoanLimitConfig       = errors.New("failed to delete loan limit config")
)

// LoanLimitConfigRepository defines the interface for loan limit config data access
type LoanLimitConfigRepository interface {
	// Create operations
	Create(ctx context.Context, config *models.LoanLimitConfig) error
	BatchCreate(ctx context.Context, configs []*models.LoanLimitConfig) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.LoanLimitConfig, error)
	GetByRiskTier(ctx context.Context, riskTier string) (*models.LoanLimitConfig, error)
	GetAll(ctx context.Context, limit, offset int) ([]*models.LoanLimitConfig, error)
	GetActive(ctx context.Context, limit, offset int) ([]*models.LoanLimitConfig, error)

	// Update operations
	Update(ctx context.Context, config *models.LoanLimitConfig) error
	Restore(ctx context.Context, id string) error

	// Delete operations
	Delete(ctx context.Context, id string) error
}

// loanLimitConfigRepository represents a repository for managing loan limit configs
type loanLimitConfigRepository struct {
	db *gorm.DB
}

// NewLoanLimitConfigRepository creates a new instance of LoanLimitConfigRepository
func NewLoanLimitConfigRepository(db *gorm.DB) (LoanLimitConfigRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &loanLimitConfigRepository{db: db}, nil
}

// --- Create Operations ---

// Create creates a new loan limit config
func (r *loanLimitConfigRepository) Create(ctx context.Context, config *models.LoanLimitConfig) error {
	result := r.db.WithContext(ctx).Create(config)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateLoanLimitConfig
	}
	return nil
}

// BatchCreate creates multiple loan limit configs in a single batch
func (r *loanLimitConfigRepository) BatchCreate(ctx context.Context, configs []*models.LoanLimitConfig) error {
	if len(configs) == 0 {
		return nil
	}

	result := r.db.WithContext(ctx).Create(configs)
	if result.Error != nil {
		log.Printf("CreateBatch: database error: %v", result.Error)
		return ErrFailedToCreateBatchLoanLimitConfigs
	}
	return nil
}

// --- Read Operations ---

// GetByID retrieves a loan limit config by its ID
func (r *loanLimitConfigRepository) GetByID(ctx context.Context, id string) (*models.LoanLimitConfig, error) {
	var config models.LoanLimitConfig
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&config)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrLoanLimitConfigNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetLoanLimitConfig
	}
	return &config, nil
}

// GetByRiskTier retrieves a loan limit config by risk tier
func (r *loanLimitConfigRepository) GetByRiskTier(ctx context.Context, riskTier string) (*models.LoanLimitConfig, error) {
	var config models.LoanLimitConfig
	result := r.db.WithContext(ctx).
		Where("risk_tier = ? AND is_active = ?", riskTier, true).
		First(&config)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrLoanLimitConfigNotFound
	}
	if result.Error != nil {
		log.Printf("GetByRiskTier: database error: %v", result.Error)
		return nil, ErrFailedToGetLoanLimitConfigByTier
	}
	return &config, nil
}

// GetAll retrieves all loan limit configs
func (r *loanLimitConfigRepository) GetAll(ctx context.Context, limit, offset int) ([]*models.LoanLimitConfig, error) {
	var configs []*models.LoanLimitConfig
	result := r.db.WithContext(ctx).
		Order("risk_tier ASC").
		Limit(limit).
		Offset(offset).
		Find(&configs)
	if result.Error != nil {
		log.Printf("GetAll: database error: %v", result.Error)
		return nil, ErrFailedToGetAllLoanLimitConfigs
	}
	return configs, nil
}

// GetActive retrieves all active loan limit configs
func (r *loanLimitConfigRepository) GetActive(ctx context.Context, limit, offset int) ([]*models.LoanLimitConfig, error) {
	var configs []*models.LoanLimitConfig
	result := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("risk_tier ASC").
		Limit(limit).
		Offset(offset).
		Find(&configs)
	if result.Error != nil {
		log.Printf("GetActive: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveLoanLimitConfigs
	}
	return configs, nil
}

// --- Update Operations ---

// Update updates a loan limit config
func (r *loanLimitConfigRepository) Update(ctx context.Context, config *models.LoanLimitConfig) error {
	result := r.db.WithContext(ctx).
		Model(config).
		Where("id = ?", config.ID).
		Updates(map[string]interface{}{
			"risk_tier":              config.RiskTier,
			"min_loan_amount":        config.MinLoanAmount,
			"max_loan_amount":        config.MaxLoanAmount,
			"income_multiplier_bps":  config.IncomeMultiplierBps,
			"max_concurrent_loans":   config.MaxConcurrentLoans,
			"max_loan_duration_days": config.MaxLoanDurationDays,
			"interest_rate_bps":      config.InterestRateBps,
			"late_fee_bps":           config.LateFeeBps,
			"default_penalty_bps":    config.DefaultPenaltyBps,
			"is_active":              config.IsActive,
			"updated_by":             config.UpdatedBy,
			"updated_at":             time.Now(),
		})
	if result.RowsAffected == 0 {
		return ErrLoanLimitConfigNotFound
	}
	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateLoanLimitConfig
	}
	return nil
}

// Restore restores a loan limit config by ID
func (r *loanLimitConfigRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.LoanLimitConfig{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrLoanLimitConfigNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreLoanLimitConfig
	}
	return nil
}

// --- Delete Operations ---

// Delete deletes a loan limit config by ID
func (r *loanLimitConfigRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.LoanLimitConfig{})
	if result.RowsAffected == 0 {
		return ErrLoanLimitConfigNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteLoanLimitConfig
	}
	return nil
}
