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

// Common errors for CashflowAnalysisRepository
var (
	ErrCashflowAnalysisNotFound               = errors.New("cashflow analysis not found")
	ErrInvalidCashflowAnalysis                = errors.New("invalid cashflow analysis data")
	ErrFailedToCreateCashflowAnalysis         = errors.New("failed to create cashflow analysis")
	ErrFailedToCreateBatchCashflowAnalysis    = errors.New("failed to bathc create cashflow analysis")
	ErrFailedToGetCashflowAnalysis            = errors.New("failed to get cashflow analysis")
	ErrFailedToGetLatestCashflowAnalysis      = errors.New("failed to get latest cashflow analysis")
	ErrFailedToGetAllCashflowAnalyses         = errors.New("failed to get all cashflow analyses")
	ErrFailedToGetCashflowAnalysisWithUser    = errors.New("failed to get cashflow analysis with user")
	ErrFailedToGetCashflowAnalysisByPeriod    = errors.New("failed to get cashflow analysis by period")
	ErrFailedToGetCashflowAnalysisCovering    = errors.New("failed to get cashflow analysis covering date")
	ErrFailedToGetNonStaleCashflowAnalysis    = errors.New("failed to get non-stale cashflow analysis")
	ErrFailedToGetPositiveCashflow            = errors.New("failed to get positive cashflow analyses")
	ErrFailedToGetByMinSavingsRate            = errors.New("failed to get analyses by minimum savings rate")
	ErrFailedToGetByIncomeStabilityScore      = errors.New("failed to get analyses by income stability score")
	ErrFailedToGetAverageIncome               = errors.New("failed to get average income")
	ErrFailedToGetAverageExpenses             = errors.New("failed to get average expenses")
	ErrFailedToGetTotalTransactionCount       = errors.New("failed to get total transaction count")
	ErrFailedToCountCashflowAnalyses          = errors.New("failed to count cashflow analyses")
	ErrFailedToUpdateCashflowAnalysis         = errors.New("failed to update cashflow analysis")
	ErrFailedToRestoreCashflowAnalysis        = errors.New("failed to restore cashflow analysis")
	ErrFailedToDeleteCashflowAnalysis         = errors.New("failed to delete cashflow analysis")
	ErrFailedToDeleteCashflowAnalysesByUserID = errors.New("failed to delete cashflow analyses by user ID")
	ErrFailedToGetTwoMostRecent               = errors.New("failed to get two most recent analyses")
)

// CashflowAnalysisRepository defines the interface for cash flow data access
type CashflowAnalysisRepository interface {
	// Create operations
	Create(ctx context.Context, analysis *models.CashflowAnalysis) error
	BatchCreate(ctx context.Context, analyses []*models.CashflowAnalysis) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.CashflowAnalysis, error)
	GetLatestByUserID(ctx context.Context, userID string) (*models.CashflowAnalysis, error)
	GetAllByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.CashflowAnalysis, error)
	GetByUserIDWithUser(ctx context.Context, userID string) (*models.CashflowAnalysis, error)
	GetByUserIDAndPeriod(ctx context.Context, userID string, start, end time.Time) (*models.CashflowAnalysis, error)
	GetByUserIDCoveringDate(ctx context.Context, userID string, date time.Time) (*models.CashflowAnalysis, error)
	GetNonStaleByUserID(ctx context.Context, userID string, maxAge time.Duration) (*models.CashflowAnalysis, error)
	GetPositiveCashflowByUserID(ctx context.Context, userID string) ([]*models.CashflowAnalysis, error)
	GetByMinSavingsRate(ctx context.Context, minRateBps int32, limit, offset int) ([]*models.CashflowAnalysis, error)
	GetByIncomeStabilityScore(ctx context.Context, minScore int, limit, offset int) ([]*models.CashflowAnalysis, error)
	GetTwoMostRecentByUserID(ctx context.Context, userID string) ([]*models.CashflowAnalysis, error)

	// Aggregation operations
	GetAverageIncomeByUserID(ctx context.Context, userID string) (int64, error)
	GetAverageExpensesByUserID(ctx context.Context, userID string) (int64, error)
	GetTotalTransactionCountByUserID(ctx context.Context, userID string) (int64, error)
	CountByUserID(ctx context.Context, userID string) (int64, error)

	// Update operations
	Update(ctx context.Context, analysis *models.CashflowAnalysis) error
	Restore(ctx context.Context, id string) error

	// Delete operations
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error
}

// cashflowAnalysisRepository represents a repository for cash flow analysis.
type cashflowAnalysisRepository struct {
	db *gorm.DB
}

// NewCashflowAnalysisRepository creates a new CashflowAnalysisRepository instance.
func NewCashflowAnalysisRepository(db *gorm.DB) (CashflowAnalysisRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &cashflowAnalysisRepository{
		db: db,
	}, nil
}

// --- Create Operations ---

// Create creates a new cash flow analysis record.
func (r *cashflowAnalysisRepository) Create(ctx context.Context, analysis *models.CashflowAnalysis) error {
	if !analysis.IsValid() {
		return ErrInvalidCashflowAnalysis
	}

	// Calculate derived fields
	analysis.CalculateNetCashflow()
	analysis.CalculateSavingsRate()
	analysis.CalculateMonthlyAverages()
	analysis.CalculatedAt = time.Now()

	result := r.db.WithContext(ctx).Create(analysis)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateCashflowAnalysis
	}
	return nil
}

// BatchCreate creates multiple cash flow analysis records in a transaction.
func (r *cashflowAnalysisRepository) BatchCreate(ctx context.Context, analyses []*models.CashflowAnalysis) error {
	if len(analyses) == 0 {
		return nil
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, analysis := range analyses {
			if !analysis.IsValid() {
				return ErrInvalidCashflowAnalysis
			}
			analysis.CalculateNetCashflow()
			analysis.CalculateSavingsRate()
			analysis.CalculateMonthlyAverages()
			analysis.CalculatedAt = time.Now()
		}
		return tx.Create(analyses).Error
	})
	if err != nil {
		log.Printf("BatchCreate: database error: %v", err)
		if errors.Is(err, ErrInvalidCashflowAnalysis) {
			return err
		}
		return ErrFailedToCreateBatchCashflowAnalysis
	}
	return nil
}

// --- Read Operations ---

// GetByID retrieves a cash flow analysis by ID.
func (r *cashflowAnalysisRepository) GetByID(ctx context.Context, id string) (*models.CashflowAnalysis, error) {
	var analysis models.CashflowAnalysis
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&analysis)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrCashflowAnalysisNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetCashflowAnalysis
	}
	return &analysis, nil
}

// GetByUserID retrieves the most recent cash flow analysis for a user.
func (r *cashflowAnalysisRepository) GetLatestByUserID(ctx context.Context, userID string) (*models.CashflowAnalysis, error) {
	var analysis models.CashflowAnalysis
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("calculated_at DESC").
		First(&analysis)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrCashflowAnalysisNotFound
	}
	if result.Error != nil {
		log.Printf("GetLatestByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetLatestCashflowAnalysis
	}
	return &analysis, nil
}

// GetAllByUserID retrieves all cash flow analyses for a user.
func (r *cashflowAnalysisRepository) GetAllByUserID(ctx context.Context, userID string, offset int, limit int) ([]*models.CashflowAnalysis, error) {
	var analyses []*models.CashflowAnalysis
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("calculated_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&analyses)

	if result.Error != nil {
		log.Printf("GetAllByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetAllCashflowAnalyses
	}
	return analyses, nil
}

// GetByUserIDWithUser retrieves cash flow analysis with preloaded user.
func (r *cashflowAnalysisRepository) GetByUserIDWithUser(ctx context.Context, userID string) (*models.CashflowAnalysis, error) {
	var analysis models.CashflowAnalysis
	result := r.db.WithContext(ctx).
		Preload("User").
		Where("user_id = ?", userID).
		Order("calculated_at DESC").
		First(&analysis)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrCashflowAnalysisNotFound
	}
	if result.Error != nil {
		log.Printf("GetByUserIDWithUser: database error: %v", result.Error)
		return nil, ErrFailedToGetCashflowAnalysisWithUser
	}
	return &analysis, nil
}

// --- Period-Based Queries ---

// GetByUserIDAndPeriod retrieves analysis for a specific time period.
func (r *cashflowAnalysisRepository) GetByUserIDAndPeriod(ctx context.Context, userID string, start, end time.Time) (*models.CashflowAnalysis, error) {
	var analysis models.CashflowAnalysis
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND analysis_period_start = ? AND analysis_period_end = ?", userID, start, end).
		First(&analysis)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrCashflowAnalysisNotFound
	}
	if result.Error != nil {
		log.Printf("GetByUserIDAndPeriod: database error: %v", result.Error)
		return nil, ErrFailedToGetCashflowAnalysisByPeriod
	}
	return &analysis, nil
}

// GetByUserIDCoveringDate retrieves analysis that covers a specific date.
func (r *cashflowAnalysisRepository) GetByUserIDCoveringDate(ctx context.Context, userID string, date time.Time) (*models.CashflowAnalysis, error) {
	var analysis models.CashflowAnalysis
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND analysis_period_start <= ? AND analysis_period_end >= ?", userID, date, date).
		Order("calculated_at DESC").
		First(&analysis)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrCashflowAnalysisNotFound
	}
	if result.Error != nil {
		log.Printf("GetByUserIDCoveringDate: database error: %v", result.Error)
		return nil, ErrFailedToGetCashflowAnalysisCovering
	}
	return &analysis, nil
}

// GetNonStaleByUserID retrieves the most recent non-stale analysis.
func (r *cashflowAnalysisRepository) GetNonStaleByUserID(ctx context.Context, userID string, maxAge time.Duration) (*models.CashflowAnalysis, error) {
	var analysis models.CashflowAnalysis
	cutoff := time.Now().Add(-maxAge)

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND calculated_at >= ?", userID, cutoff).
		Order("calculated_at DESC").
		First(&analysis)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrCashflowAnalysisNotFound
	}
	if result.Error != nil {
		log.Printf("GetNonStaleByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetNonStaleCashflowAnalysis
	}
	return &analysis, nil
}

// --- Financial Health Queries ---

// GetPositiveCashflowByUserID retrieves all analyses with positive cashflow.
// Uses the model's IsPositiveCashflow logic at database level.
func (r *cashflowAnalysisRepository) GetPositiveCashflowByUserID(ctx context.Context, userID string) ([]*models.CashflowAnalysis, error) {
	var analyses []*models.CashflowAnalysis
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND net_cashflow > 0", userID).
		Order("calculated_at DESC").
		Find(&analyses)

	if result.Error != nil {
		log.Printf("GetPositiveCashflowByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetPositiveCashflow
	}
	return analyses, nil
}

// GetByMinSavingsRate retrieves analyses with savings rate above threshold (in basis points).
func (r *cashflowAnalysisRepository) GetByMinSavingsRate(ctx context.Context, minRateBps int32, limit, offset int) ([]*models.CashflowAnalysis, error) {
	var analyses []*models.CashflowAnalysis
	result := r.db.WithContext(ctx).
		Where("savings_rate_bps >= ?", minRateBps).
		Order("savings_rate_bps DESC").
		Limit(limit).
		Offset(offset).
		Find(&analyses)

	if result.Error != nil {
		log.Printf("GetByMinSavingsRate: database error: %v", result.Error)
		return nil, ErrFailedToGetByMinSavingsRate
	}
	return analyses, nil
}

// GetByIncomeStabilityScore retrieves analyses with income stability above threshold.
func (r *cashflowAnalysisRepository) GetByIncomeStabilityScore(ctx context.Context, minScore int, limit, offset int) ([]*models.CashflowAnalysis, error) {
	var analyses []*models.CashflowAnalysis
	result := r.db.WithContext(ctx).
		Where("income_stability_score >= ?", minScore).
		Order("income_stability_score DESC").
		Limit(limit).
		Offset(offset).
		Find(&analyses)

	if result.Error != nil {
		log.Printf("GetByIncomeStabilityScore: database error: %v", result.Error)
		return nil, ErrFailedToGetByIncomeStabilityScore
	}
	return analyses, nil
}

// --- Aggregation Queries ---

// GetAverageIncomeByUserID calculates average total income across all analyses (in smallest unit).
func (r *cashflowAnalysisRepository) GetAverageIncomeByUserID(ctx context.Context, userID string) (int64, error) {
	var avg int64
	result := r.db.WithContext(ctx).
		Model(&models.CashflowAnalysis{}).
		Where("user_id = ?", userID).
		Select("COALESCE(AVG(total_income), 0)").
		Scan(&avg)

	if result.Error != nil {
		log.Printf("GetAverageIncomeByUserID: database error: %v", result.Error)
		return 0, ErrFailedToGetAverageIncome
	}
	return avg, nil
}

// GetAverageExpensesByUserID calculates average total expenses across all analyses (in smallest unit).
func (r *cashflowAnalysisRepository) GetAverageExpensesByUserID(ctx context.Context, userID string) (int64, error) {
	var avg int64
	result := r.db.WithContext(ctx).
		Model(&models.CashflowAnalysis{}).
		Where("user_id = ?", userID).
		Select("COALESCE(AVG(total_expenses), 0)").
		Scan(&avg)

	if result.Error != nil {
		log.Printf("GetAverageExpensesByUserID: database error: %v", result.Error)
		return 0, ErrFailedToGetAverageExpenses
	}
	return avg, nil
}

// GetTotalTransactionCountByUserID sums transaction counts across all analyses.
func (r *cashflowAnalysisRepository) GetTotalTransactionCountByUserID(ctx context.Context, userID string) (int64, error) {
	var total int64
	result := r.db.WithContext(ctx).
		Model(&models.CashflowAnalysis{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(transaction_count), 0)").
		Scan(&total)

	if result.Error != nil {
		log.Printf("GetTotalTransactionCountByUserID: database error: %v", result.Error)
		return 0, ErrFailedToGetTotalTransactionCount
	}
	return total, nil
}

// CountByUserID counts the number of analyses for a user.
func (r *cashflowAnalysisRepository) CountByUserID(ctx context.Context, userID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.CashflowAnalysis{}).
		Where("user_id = ?", userID).
		Count(&count)

	if result.Error != nil {
		log.Printf("CountByUserID: database error: %v", result.Error)
		return 0, ErrFailedToCountCashflowAnalyses
	}
	return count, nil
}

// --- Update Operations ---

// Update updates an existing cash flow analysis.
func (r *cashflowAnalysisRepository) Update(ctx context.Context, analysis *models.CashflowAnalysis) error {
	if !analysis.IsValid() {
		return ErrInvalidCashflowAnalysis
	}

	// Recalculate derived fields
	analysis.CalculateNetCashflow()
	analysis.CalculateSavingsRate()
	analysis.CalculateMonthlyAverages()

	result := r.db.WithContext(ctx).
		Model(analysis).
		Updates(map[string]interface{}{
			"analysis_period_start":    analysis.AnalysisPeriodStart,
			"analysis_period_end":      analysis.AnalysisPeriodEnd,
			"total_income":             analysis.TotalIncome,
			"total_expenses":           analysis.TotalExpenses,
			"net_cashflow":             analysis.NetCashflow,
			"avg_monthly_income":       analysis.AvgMonthlyIncome,
			"avg_monthly_expenses":     analysis.AvgMonthlyExpenses,
			"income_stability_score":   analysis.IncomeStabilityScore,
			"expense_regularity_score": analysis.ExpenseRegularityScore,
			"savings_rate_bps":         analysis.SavingsRateBps,
			"transaction_count":        analysis.TransactionCount,
			"unique_income_sources":    analysis.UniqueIncomeSources,
			"income_sources":           analysis.IncomeSources,
			"expense_categories":       analysis.ExpenseCategories,
			"seasonal_pattern":         analysis.SeasonalPattern,
			"risk_flags":               analysis.RiskFlags,
			"calculated_at":            time.Now(),
		})

	if result.RowsAffected == 0 {
		return ErrCashflowAnalysisNotFound
	}
	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateCashflowAnalysis
	}
	return nil
}

// Restore restores a cash flow analysis by ID.
func (r *cashflowAnalysisRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.CashflowAnalysis{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrCashflowAnalysisNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreCashflowAnalysis
	}
	return nil
}

// --- Delete Operations ---

// Delete removes a cash flow analysis by ID.
func (r *cashflowAnalysisRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.CashflowAnalysis{})

	if result.RowsAffected == 0 {
		return ErrCashflowAnalysisNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteCashflowAnalysis
	}
	return nil
}

// DeleteByUserID removes all cash flow analyses for a user.
func (r *cashflowAnalysisRepository) DeleteByUserID(ctx context.Context, userID string) error {
	result := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.CashflowAnalysis{})
	if result.Error != nil {
		log.Printf("DeleteByUserID: database error: %v", result.Error)
		return ErrFailedToDeleteCashflowAnalysesByUserID
	}
	return nil
}

// --- Comparison operations ---

// GetTwoMostRecentByUserID retrieves two most recent analyses for comparison.
func (r *cashflowAnalysisRepository) GetTwoMostRecentByUserID(ctx context.Context, userID string) ([]*models.CashflowAnalysis, error) {
	var analyses []*models.CashflowAnalysis
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("calculated_at DESC").
		Limit(2).
		Find(&analyses)

	if result.Error != nil {
		log.Printf("GetTwoMostRecentByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetTwoMostRecent
	}
	return analyses, nil
}
