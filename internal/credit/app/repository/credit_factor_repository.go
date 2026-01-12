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

// Common errors for CreditFactorRepository
var (
	ErrCreditFactorNotFound                  = errors.New("credit factor not found")
	ErrInvalidCreditFactor                   = errors.New("invalid credit factor: values out of acceptable range")
	ErrCreditFactorAlreadyExists             = errors.New("credit factor already exists for this user and score")
	ErrFailedToCreateCreditFactor            = errors.New("failed to create credit factor")
	ErrFailedToCreateBatchCreditFactor       = errors.New("failed to batch create credit factors")
	ErrFailedToGetCreditFactor               = errors.New("failed to get credit factor")
	ErrFailedToGetCreditFactorsByUserID      = errors.New("failed to get credit factors by user ID")
	ErrFailedToGetCreditFactorsByScoreID     = errors.New("failed to get credit factors by credit score ID")
	ErrFailedToGetCreditFactorByName         = errors.New("failed to get credit factor by factor name")
	ErrFailedToGetLatestCreditFactors        = errors.New("failed to get latest credit factors")
	ErrFailedToGetCreditFactorsByName        = errors.New("failed to get credit factors by name")
	ErrFailedToGetCreditFactorsByDate        = errors.New("failed to get credit factors by date range")
	ErrFailedToGetCreditFactorsBySource      = errors.New("failed to get credit factors by data source")
	ErrFailedToUpdateCreditFactor            = errors.New("failed to update credit factor")
	ErrFailedToDeleteCreditFactor            = errors.New("failed to delete credit factor")
	ErrFailedToDeleteCreditFactorsByUser     = errors.New("failed to delete credit factors by user ID")
	ErrFailedToDeleteCreditFactorsByScore    = errors.New("failed to delete credit factors by credit score ID")
	ErrFailedToGetTotalWeightedScore         = errors.New("failed to get total weighted score")
	ErrFailedToGetAverageFactorValue         = errors.New("failed to get average factor value")
	ErrFailedToCountCreditFactors            = errors.New("failed to count credit factors")
	ErrFailedToRestoreCreditFactor           = errors.New("failed to restore credit factor")
	ErrFailedToGetCreditFactorWithRelations  = errors.New("failed to get credit factor with relations")
	ErrFailedToGetCreditFactorsWithRelations = errors.New("failed to get credit factors with relations")
	ErrFailedToFindWithFilter                = errors.New("failed to find credit factors with filter")
	ErrFailedToUpsertCreditFactor            = errors.New("failed to upsert credit factor")
)

// CreditFactorRepository defines the interface for credit factor data access
type CreditFactorRepository interface {
	// Create operations
	Create(ctx context.Context, factor *models.CreditFactor) error
	BatchCreate(ctx context.Context, factors []*models.CreditFactor) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.CreditFactor, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.CreditFactor, error)
	GetByCreditScoreID(ctx context.Context, creditScoreID string, limit, offset int) ([]*models.CreditFactor, error)
	GetByUserAndFactorName(ctx context.Context, userID, factorName string) (*models.CreditFactor, error)
	GetLatestByUserID(ctx context.Context, userID string) ([]*models.CreditFactor, error)

	// Update operations
	Update(ctx context.Context, factor *models.CreditFactor) error
	Restore(ctx context.Context, id string) error

	// Delete operations
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error
	DeleteByCreditScoreID(ctx context.Context, creditScoreID string) error

	// Query operations
	GetByFactorName(ctx context.Context, factorName string, limit, offset int) ([]*models.CreditFactor, error)
	GetByDateRange(ctx context.Context, userID string, start, end time.Time) ([]*models.CreditFactor, error)
	GetByDataSource(ctx context.Context, dataSource string, limit, offset int) ([]*models.CreditFactor, error)

	// Aggregation operations
	GetTotalWeightedScoreByUser(ctx context.Context, userID string) (int32, error)
	GetAverageFactorValueByName(ctx context.Context, factorName string) (int32, error)
	CountByUserID(ctx context.Context, userID string) (int64, error)

	// Preload operations
	GetByIDWithRelations(ctx context.Context, id string) (*models.CreditFactor, error)
	GetByUserIDWithRelations(ctx context.Context, userID string) ([]*models.CreditFactor, error)
}

// creditFactorRepository implements CreditFactorRepository using GORM
type creditFactorRepository struct {
	db *gorm.DB
}

// NewCreditFactorRepository creates a new instance of CreditFactorRepository
func NewCreditFactorRepository(db *gorm.DB) (CreditFactorRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &creditFactorRepository{db: db}, nil
}

// --- Create Operations ---

// Create inserts a new credit factor into the database
func (r *creditFactorRepository) Create(ctx context.Context, factor *models.CreditFactor) error {
	if !factor.IsValid() {
		return ErrInvalidCreditFactor
	}

	factor.CalculateWeightedScore()
	factor.CalculatedAt = time.Now()

	result := r.db.WithContext(ctx).Create(factor)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateCreditFactor
	}
	return nil
}

// BatchCreate inserts multiple credit factors in a single transaction
func (r *creditFactorRepository) BatchCreate(ctx context.Context, factors []*models.CreditFactor) error {
	if len(factors) == 0 {
		return nil
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, factor := range factors {
			if !factor.IsValid() {
				return ErrInvalidCreditFactor
			}
			factor.CalculateWeightedScore()
			factor.CalculatedAt = time.Now()
		}

		if err := tx.Create(factors).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		log.Printf("BatchCreate: database error: %v", err)
		if errors.Is(err, ErrInvalidCreditFactor) {
			return err
		}
		return ErrFailedToCreateBatchCreditFactor
	}
	return nil
}

// --- Read Operations ---

// GetByID retrieves a credit factor by its ID
func (r *creditFactorRepository) GetByID(ctx context.Context, id string) (*models.CreditFactor, error) {
	var factor models.CreditFactor

	result := r.db.WithContext(ctx).Where("id = ?", id).First(&factor)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrCreditFactorNotFound
		}
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditFactor
	}

	return &factor, nil
}

// GetByUserID retrieves all credit factors for a specific user
func (r *creditFactorRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.CreditFactor, error) {
	var factors []*models.CreditFactor

	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("calculated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&factors)

	if result.Error != nil {
		log.Printf("GetByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditFactorsByUserID
	}

	return factors, nil
}

// GetByCreditScoreID retrieves all credit factors linked to a specific credit score
func (r *creditFactorRepository) GetByCreditScoreID(ctx context.Context, creditScoreID string, limit, offset int) ([]*models.CreditFactor, error) {
	var factors []*models.CreditFactor

	result := r.db.WithContext(ctx).
		Where("credit_score_id = ?", creditScoreID).
		Order("factor_weight_bps DESC").
		Limit(limit).
		Offset(offset).
		Find(&factors)

	if result.Error != nil {
		log.Printf("GetByCreditScoreID: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditFactorsByScoreID
	}

	return factors, nil
}

// GetByUserAndFactorName retrieves a specific factor type for a user
func (r *creditFactorRepository) GetByUserAndFactorName(ctx context.Context, userID, factorName string) (*models.CreditFactor, error) {
	var factor models.CreditFactor

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND factor_name = ?", userID, factorName).
		Order("calculated_at DESC").
		First(&factor)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrCreditFactorNotFound
		}
		log.Printf("GetByUserAndFactorName: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditFactorByName
	}

	return &factor, nil
}

// GetLatestByUserID retrieves the most recent credit factors for each factor type for a user
func (r *creditFactorRepository) GetLatestByUserID(ctx context.Context, userID string) ([]*models.CreditFactor, error) {
	var factors []*models.CreditFactor

	// Subquery to get the latest calculated_at for each factor_name
	subQuery := r.db.WithContext(ctx).
		Model(&models.CreditFactor{}).
		Select("factor_name, MAX(calculated_at) as max_calculated_at").
		Where("user_id = ?", userID).
		Group("factor_name")

	result := r.db.WithContext(ctx).
		Joins("JOIN (?) AS latest ON credit_factors.factor_name = latest.factor_name AND credit_factors.calculated_at = latest.max_calculated_at", subQuery).
		Where("credit_factors.user_id = ?", userID).
		Find(&factors)

	if result.Error != nil {
		log.Printf("GetLatestByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetLatestCreditFactors
	}

	return factors, nil
}

// --- Update Operations ---

// Update updates an existing credit factor
func (r *creditFactorRepository) Update(ctx context.Context, factor *models.CreditFactor) error {
	if !factor.IsValid() {
		return ErrInvalidCreditFactor
	}

	factor.CalculateWeightedScore()

	result := r.db.WithContext(ctx).
		Model(factor).
		Updates(map[string]interface{}{
			"factor_value_bps":   factor.FactorValueBps,
			"factor_weight_bps":  factor.FactorWeightBps,
			"weighted_score_bps": factor.WeightedScoreBps,
			"calculation_method": factor.CalculationMethod,
			"data_source":        factor.DataSource,
			"updated_at":         time.Now(),
		})

	if result.RowsAffected == 0 {
		return ErrCreditFactorNotFound
	}

	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateCreditFactor
	}

	return nil
}

// Restore restores a credit factor by ID.
func (r *creditFactorRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.CreditFactor{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrCreditFactorNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreCreditFactor
	}
	return nil
}

// --- Delete Operations ---

// Delete removes a credit factor by ID
func (r *creditFactorRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.CreditFactor{})

	if result.RowsAffected == 0 {
		return ErrCreditFactorNotFound
	}

	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteCreditFactor
	}

	return nil
}

// DeleteByUserID removes all credit factors for a specific user
func (r *creditFactorRepository) DeleteByUserID(ctx context.Context, userID string) error {
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&models.CreditFactor{})

	if result.RowsAffected == 0 {
		return ErrCreditFactorNotFound
	}

	if result.Error != nil {
		log.Printf("DeleteByUserID: database error: %v", result.Error)
		return ErrFailedToDeleteCreditFactorsByUser
	}

	return nil
}

// DeleteByCreditScoreID removes all credit factors linked to a specific credit score
func (r *creditFactorRepository) DeleteByCreditScoreID(ctx context.Context, creditScoreID string) error {
	result := r.db.WithContext(ctx).
		Where("credit_score_id = ?", creditScoreID).
		Delete(&models.CreditFactor{})

	if result.RowsAffected == 0 {
		return ErrCreditFactorNotFound
	}

	if result.Error != nil {
		log.Printf("DeleteByCreditScoreID: database error: %v", result.Error)
		return ErrFailedToDeleteCreditFactorsByScore
	}

	return nil
}

// --- Query Operations ---

// GetByFactorName retrieves all credit factors of a specific type with pagination
func (r *creditFactorRepository) GetByFactorName(ctx context.Context, factorName string, limit, offset int) ([]*models.CreditFactor, error) {
	var factors []*models.CreditFactor

	result := r.db.WithContext(ctx).
		Where("factor_name = ?", factorName).
		Order("calculated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&factors)

	if result.Error != nil {
		log.Printf("GetByFactorName: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditFactorsByName
	}

	return factors, nil
}

// GetByDateRange retrieves credit factors for a user within a specific date range
func (r *creditFactorRepository) GetByDateRange(ctx context.Context, userID string, start, end time.Time) ([]*models.CreditFactor, error) {
	var factors []*models.CreditFactor

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND calculated_at BETWEEN ? AND ?", userID, start, end).
		Order("calculated_at DESC").
		Find(&factors)

	if result.Error != nil {
		log.Printf("GetByDateRange: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditFactorsByDate
	}

	return factors, nil
}

// GetByDataSource retrieves all credit factors from a specific data source with pagination
func (r *creditFactorRepository) GetByDataSource(ctx context.Context, dataSource string, limit, offset int) ([]*models.CreditFactor, error) {
	var factors []*models.CreditFactor

	result := r.db.WithContext(ctx).
		Where("data_source = ?", dataSource).
		Order("calculated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&factors)

	if result.Error != nil {
		log.Printf("GetByDataSource: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditFactorsBySource
	}

	return factors, nil
}

// --- Aggregation Operations ---

// GetTotalWeightedScoreByUser calculates the sum of all weighted scores for a user
func (r *creditFactorRepository) GetTotalWeightedScoreByUser(ctx context.Context, userID string) (int32, error) {
	var total int32

	result := r.db.WithContext(ctx).
		Model(&models.CreditFactor{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(weighted_score_bps), 0)").
		Scan(&total)

	if result.Error != nil {
		log.Printf("GetTotalWeightedScoreByUser: database error: %v", result.Error)
		return 0, ErrFailedToGetTotalWeightedScore
	}

	return total, nil
}

// GetAverageFactorValueByName calculates the average value for a specific factor type
func (r *creditFactorRepository) GetAverageFactorValueByName(ctx context.Context, factorName string) (int32, error) {
	var avg int32

	result := r.db.WithContext(ctx).
		Model(&models.CreditFactor{}).
		Where("factor_name = ?", factorName).
		Select("COALESCE(AVG(factor_value_bps), 0)").
		Scan(&avg)

	if result.Error != nil {
		log.Printf("GetAverageFactorValueByName: database error: %v", result.Error)
		return 0, ErrFailedToGetAverageFactorValue
	}

	return avg, nil
}

// CountByUserID counts the number of credit factors for a specific user
func (r *creditFactorRepository) CountByUserID(ctx context.Context, userID string) (int64, error) {
	var count int64

	result := r.db.WithContext(ctx).
		Model(&models.CreditFactor{}).
		Where("user_id = ?", userID).
		Count(&count)

	if result.Error != nil {
		log.Printf("CountByUserID: database error: %v", result.Error)
		return 0, ErrFailedToCountCreditFactors
	}

	return count, nil
}

// --- Preload Operations ---

// GetByIDWithRelations retrieves a credit factor with its related User and CreditScore
func (r *creditFactorRepository) GetByIDWithRelations(ctx context.Context, id string) (*models.CreditFactor, error) {
	var factor models.CreditFactor

	result := r.db.WithContext(ctx).
		Preload("User").
		Preload("CreditScore").
		Where("id = ?", id).
		First(&factor)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrCreditFactorNotFound
		}
		log.Printf("GetByIDWithRelations: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditFactorWithRelations
	}

	return &factor, nil
}

// GetByUserIDWithRelations retrieves all credit factors for a user with related entities
func (r *creditFactorRepository) GetByUserIDWithRelations(ctx context.Context, userID string) ([]*models.CreditFactor, error) {
	var factors []*models.CreditFactor

	result := r.db.WithContext(ctx).
		Preload("User").
		Preload("CreditScore").
		Where("user_id = ?", userID).
		Order("calculated_at DESC").
		Find(&factors)

	if result.Error != nil {
		log.Printf("GetByUserIDWithRelations: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditFactorsWithRelations
	}

	return factors, nil
}

// --- Special Operations ---

// CreditFactorFilter provides filtering options for complex queries
type CreditFactorFilter struct {
	UserID        *string
	CreditScoreID *string
	FactorNames   []string
	DataSources   []string
	MinValue      *int32
	MaxValue      *int32
	StartDate     *time.Time
	EndDate       *time.Time
	Limit         int
	Offset        int
	OrderBy       string
	OrderDesc     bool
}

// FindWithFilter retrieves credit factors based on multiple filter criteria
func (r *creditFactorRepository) FindWithFilter(ctx context.Context, filter CreditFactorFilter) ([]*models.CreditFactor, int64, error) {
	var factors []*models.CreditFactor
	var total int64

	query := r.db.WithContext(ctx).Model(&models.CreditFactor{})

	// Apply filters
	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}

	if filter.CreditScoreID != nil {
		query = query.Where("credit_score_id = ?", *filter.CreditScoreID)
	}

	if len(filter.FactorNames) > 0 {
		query = query.Where("factor_name IN ?", filter.FactorNames)
	}

	if len(filter.DataSources) > 0 {
		query = query.Where("data_source IN ?", filter.DataSources)
	}

	if filter.MinValue != nil {
		query = query.Where("factor_value_bps >= ?", *filter.MinValue)
	}

	if filter.MaxValue != nil {
		query = query.Where("factor_value_bps <= ?", *filter.MaxValue)
	}

	if filter.StartDate != nil {
		query = query.Where("calculated_at >= ?", *filter.StartDate)
	}

	if filter.EndDate != nil {
		query = query.Where("calculated_at <= ?", *filter.EndDate)
	}

	// Get total count before pagination
	if err := query.Count(&total).Error; err != nil {
		log.Printf("FindWithFilter: database error counting: %v", err)
		return nil, 0, ErrFailedToFindWithFilter
	}

	// Apply ordering
	orderBy := "calculated_at"
	if filter.OrderBy != "" {
		orderBy = filter.OrderBy
	}

	if filter.OrderDesc {
		query = query.Order(orderBy + " DESC")
	} else {
		query = query.Order(orderBy + " ASC")
	}

	// Apply pagination
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	// Execute query
	if err := query.Find(&factors).Error; err != nil {
		log.Printf("FindWithFilter: database error finding: %v", err)
		return nil, 0, ErrFailedToFindWithFilter
	}

	return factors, total, nil
}

// UpsertByUserAndFactorName creates or updates a credit factor based on user and factor name
func (r *creditFactorRepository) UpsertByUserAndFactorName(ctx context.Context, factor *models.CreditFactor) error {
	if !factor.IsValid() {
		return ErrInvalidCreditFactor
	}

	factor.CalculateWeightedScore()

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND factor_name = ?", factor.UserID, factor.FactorName).
		Assign(map[string]interface{}{
			"factor_value_bps":   factor.FactorValueBps,
			"factor_weight_bps":  factor.FactorWeightBps,
			"weighted_score_bps": factor.WeightedScoreBps,
			"credit_score_id":    factor.CreditScoreID,
			"calculation_method": factor.CalculationMethod,
			"data_source":        factor.DataSource,
			"calculated_at":      time.Now(),
		}).
		FirstOrCreate(factor)

	if result.Error != nil {
		log.Printf("UpsertByUserAndFactorName: database error: %v", result.Error)
		return ErrFailedToUpsertCreditFactor
	}

	return nil
}
