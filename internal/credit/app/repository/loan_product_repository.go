package repository

import (
	"context"
	"errors"
	"log"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	"gorm.io/gorm"
)

// Common Errors for LoanProductRepository
var (
	ErrLoanProductNotFound             = errors.New("loan product not found")
	ErrFailedToCreateLoanProduct       = errors.New("failed to create loan product")
	ErrFailedToCreateBatchLoanProducts = errors.New("failed to create batch loan products")
	ErrFailedToGetLoanProduct          = errors.New("failed to get loan product")
	ErrFailedToGetActiveLoanProducts   = errors.New("failed to get active loan products")
	ErrFailedToGetAllLoanProducts      = errors.New("failed to get all loan products")
	ErrFailedToUpdateLoanProduct       = errors.New("failed to update loan product")
	ErrFailedToRestoreLoanProduct      = errors.New("failed to restore loan product")
	ErrFailedToDeleteLoanProduct       = errors.New("failed to delete loan product")
)

// LoanProductRepository is an interface for accessing loanProductRepository data.
type LoanProductRepository interface {
	// Create operations
	Create(ctx context.Context, product *models.LoanProduct) error
	BatchCreate(ctx context.Context, products []*models.LoanProduct) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.LoanProduct, error)
	GetAllActive(ctx context.Context, limit, offset int) ([]*models.LoanProduct, error)
	GetAll(ctx context.Context, limit, offset int) ([]*models.LoanProduct, error)

	// Update operations
	Update(ctx context.Context, product *models.LoanProduct) error
	Restore(ctx context.Context, id string) error

	// Delete operations
	Delete(ctx context.Context, id string) error
}

// loanProductRepository represents a repository for managing loan products
type loanProductRepository struct {
	db *gorm.DB
}

// NewLoanProductRepository creates a new instance of LoanProductRepository
func NewLoanProductRepository(db *gorm.DB) (LoanProductRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &loanProductRepository{db: db}, nil
}

// --- Create operations ---

// Create creates a new loan product.
func (r *loanProductRepository) Create(ctx context.Context, product *models.LoanProduct) error {
	result := r.db.WithContext(ctx).Create(product)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateLoanProduct
	}
	return nil
}

// BatchCreate creates multiple loan products in a single batch.
func (r *loanProductRepository) BatchCreate(ctx context.Context, products []*models.LoanProduct) error {
	if len(products) == 0 {
		return nil
	}

	result := r.db.WithContext(ctx).Create(products)
	if result.Error != nil {
		log.Printf("CreateBatch: database error: %v", result.Error)
		return ErrFailedToCreateBatchLoanProducts
	}
	return nil
}

// --- Read operations ---

// GetByID returns a loan product by its ID.
func (r *loanProductRepository) GetByID(ctx context.Context, id string) (*models.LoanProduct, error) {
	var product models.LoanProduct
	result := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&product)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrLoanProductNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetLoanProduct
	}
	return &product, nil
}

// GetAllActive returns all active loan products.
func (r *loanProductRepository) GetAllActive(ctx context.Context, limit int, offset int) ([]*models.LoanProduct, error) {
	var products []*models.LoanProduct
	result := r.db.WithContext(ctx).
		Where("is_active = ? AND deleted_at IS NULL", true).
		Order("priority_order ASC, name ASC").
		Offset(offset).
		Limit(limit).
		Find(&products)
	if result.Error != nil {
		log.Printf("GetAllActive: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveLoanProducts
	}
	return products, nil
}

// GetAll returns all loan products.
func (r *loanProductRepository) GetAll(ctx context.Context, limit int, offset int) ([]*models.LoanProduct, error) {
	var products []*models.LoanProduct
	result := r.db.WithContext(ctx).
		Order("priority_order ASC, name ASC").
		Offset(offset).
		Limit(limit).
		Find(&products)
	if result.Error != nil {
		log.Printf("GetAll: database error: %v", result.Error)
		return nil, ErrFailedToGetAllLoanProducts
	}
	return products, nil
}

// --- Update Operations ---

// Update updates a loan product.
func (r *loanProductRepository) Update(ctx context.Context, product *models.LoanProduct) error {
	result := r.db.WithContext(ctx).
		Model(product).
		Where("id = ? AND deleted_at IS NULL", product.ID).
		Updates(map[string]interface{}{
			"name":                        product.Name,
			"description":                 product.Description,
			"interest_rate_bps":           product.InterestRateBps,
			"interest_type":               product.InterestType,
			"origination_fee_bps":         product.OriginationFeeBps,
			"min_amount":                  product.MinAmount,
			"max_amount":                  product.MaxAmount,
			"currency":                    product.Currency,
			"min_duration_days":           product.MinDurationDays,
			"max_duration_days":           product.MaxDurationDays,
			"allowed_repayment_schedules": product.AllowedRepaymentSchedules,
			"max_credit_multiplier_bps":   product.MaxCreditMultiplierBps,
			"requires_collateral":         product.RequiresCollateral,
			"collateral_bps":              product.CollateralBps,
			"priority_order":              product.PriorityOrder,
			"is_active":                   product.IsActive,
			"updated_at":                  product.UpdatedAt,
		})
	if result.RowsAffected == 0 {
		return ErrLoanProductNotFound
	}
	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateLoanProduct
	}
	return nil
}

// Restore restores a loan product by ID.
func (r *loanProductRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.LoanProduct{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrLoanProductNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreLoanProduct
	}
	return nil
}

// --- Delete Operations ---

// Delete deletes a loan product by ID.
func (r *loanProductRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.LoanProduct{})
	if result.RowsAffected == 0 {
		return ErrLoanProductNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteLoanProduct
	}
	return nil
}
