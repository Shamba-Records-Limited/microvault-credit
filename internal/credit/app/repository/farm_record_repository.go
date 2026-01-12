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

// Farm record repository errors
var (
	ErrFarmRecordNotFound             = errors.New("farm record not found")
	ErrFailedToCreateFarmRecord       = errors.New("failed to create farm record")
	ErrFailedToBatchCreateFarmRecords = errors.New("failed to batch create farm records")
	ErrFailedToGetFarmRecord          = errors.New("failed to get farm record")
	ErrFailedToGetFarmRecords         = errors.New("failed to get farm records")
	ErrFailedToGetFarmRecordsByType   = errors.New("failed to get farm records by type")
	ErrFailedToGetFarmRecordsByDate   = errors.New("failed to get farm records by date range")
	ErrFailedToGetFarmRecordsByFilter = errors.New("failed to get farm records by filter")
	ErrFailedToGetVerifiedFarmRecords = errors.New("failed to get verified farm records")
	ErrFailedToCountFarmRecords       = errors.New("failed to count farm records")
	ErrFailedToCountVerifiedRecords   = errors.New("failed to count verified farm records")
	ErrFailedToGetUniqueCropTypes     = errors.New("failed to get unique crop types")
	ErrFailedToGetSalesSummary        = errors.New("failed to get sales summary")
	ErrFailedToGetExpensesSummary     = errors.New("failed to get expenses summary")
	ErrFailedToGetFarmRecordsByDoc    = errors.New("failed to get farm records by document")
	ErrFailedToGetLatestFarmRecords   = errors.New("failed to get latest farm records")
	ErrFailedToCheckRecentActivity    = errors.New("failed to check recent activity")
	ErrFailedToUpdateFarmRecord       = errors.New("failed to update farm record")
	ErrFailedToRestoreFarmRecord      = errors.New("failed to restore farm record")
	ErrFailedToDeleteFarmRecord       = errors.New("failed to delete farm record")
)

// FarmRecordRepository defines the interface for farm record data operations
type FarmRecordRepository interface {
	// --- Create operations ---
	Create(ctx context.Context, record *models.FarmRecord) error
	BatchCreate(ctx context.Context, records []*models.FarmRecord) error

	// --- Read operations ---
	GetByID(ctx context.Context, id string) (*models.FarmRecord, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.FarmRecord, error)
	GetByUserIDAndType(ctx context.Context, userID string, recordType string, limit, offset int) ([]*models.FarmRecord, error)
	GetByUserIDAndDateRange(ctx context.Context, userID string, startDate, endDate time.Time, limit, offset int) ([]*models.FarmRecord, error)
	GetByFilter(ctx context.Context, filter models.FarmRecordFilter, limit, offset int) ([]*models.FarmRecord, error)
	GetVerifiedByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.FarmRecord, error)
	CountByUserID(ctx context.Context, userID string) (int64, error)
	CountVerifiedByUserID(ctx context.Context, userID string) (int64, error)
	GetUniqueCropTypes(ctx context.Context, userID string) ([]string, error)
	GetSalesSummary(ctx context.Context, userID string, startDate, endDate time.Time) (int64, error)
	GetExpensesSummary(ctx context.Context, userID string, startDate, endDate time.Time) (int64, error)
	GetSummaryByUser(ctx context.Context, userID string, startDate, endDate time.Time) (*models.FarmRecordSummary, error)
	GetByDocumentID(ctx context.Context, documentID string) ([]*models.FarmRecord, error)
	GetLatestByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.FarmRecord, error)
	HasRecentActivity(ctx context.Context, userID string, daysBack int) (bool, error)

	// --- Update operations ---
	Update(ctx context.Context, record *models.FarmRecord) error
	Restore(ctx context.Context, id string) error

	// --- Delete operations ---
	Delete(ctx context.Context, id string) error
}

// farmRecordRepository handles farm record data operations
type farmRecordRepository struct {
	db *gorm.DB
}

// NewFarmRecordRepository creates a new farm record repository
func NewFarmRecordRepository(db *gorm.DB) (FarmRecordRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &farmRecordRepository{db: db}, nil
}

// --- Create operations ---

// Create creates a new farm record
func (r *farmRecordRepository) Create(ctx context.Context, record *models.FarmRecord) error {
	result := r.db.WithContext(ctx).Create(record)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateFarmRecord
	}
	return nil
}

// BatchCreate creates multiple farm records in a single batch
func (r *farmRecordRepository) BatchCreate(ctx context.Context, records []*models.FarmRecord) error {
	if len(records) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).Create(records)
	if result.Error != nil {
		log.Printf("BatchCreate: database error: %v", result.Error)
		return ErrFailedToBatchCreateFarmRecords
	}
	return nil
}

// --- Read operations ---

// GetByID retrieves a farm record by ID
func (r *farmRecordRepository) GetByID(ctx context.Context, id string) (*models.FarmRecord, error) {
	var record models.FarmRecord
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&record)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrFarmRecordNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetFarmRecord
	}
	return &record, nil
}

// GetByUserID retrieves all farm records for a user with pagination
func (r *farmRecordRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.FarmRecord, error) {
	var records []*models.FarmRecord
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("record_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&records)

	if result.Error != nil {
		log.Printf("GetByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetFarmRecords
	}
	return records, nil
}

// GetByUserIDAndType retrieves farm records by user and record type
func (r *farmRecordRepository) GetByUserIDAndType(ctx context.Context, userID string, recordType string, limit, offset int) ([]*models.FarmRecord, error) {
	var records []*models.FarmRecord
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND record_type = ?", userID, recordType).
		Order("record_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&records)

	if result.Error != nil {
		log.Printf("GetByUserIDAndType: database error: %v", result.Error)
		return nil, ErrFailedToGetFarmRecordsByType
	}
	return records, nil
}

// GetByUserIDAndDateRange retrieves farm records within a date range
func (r *farmRecordRepository) GetByUserIDAndDateRange(ctx context.Context, userID string, startDate, endDate time.Time, limit, offset int) ([]*models.FarmRecord, error) {
	var records []*models.FarmRecord
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND record_date BETWEEN ? AND ?", userID, startDate, endDate).
		Order("record_date ASC").
		Limit(limit).
		Offset(offset).
		Find(&records)

	if result.Error != nil {
		log.Printf("GetByUserIDAndDateRange: database error: %v", result.Error)
		return nil, ErrFailedToGetFarmRecordsByDate
	}
	return records, nil
}

// GetByFilter retrieves farm records based on flexible filter criteria
func (r *farmRecordRepository) GetByFilter(ctx context.Context, filter models.FarmRecordFilter, limit, offset int) ([]*models.FarmRecord, error) {
	var records []*models.FarmRecord

	query := r.db.WithContext(ctx).Where("user_id = ?", filter.UserID)

	if filter.RecordType != "" {
		query = query.Where("record_type = ?", filter.RecordType)
	}
	if filter.CropType != "" {
		query = query.Where("crop_type = ?", filter.CropType)
	}
	if filter.StartDate != nil {
		query = query.Where("record_date >= ?", *filter.StartDate)
	}
	if filter.EndDate != nil {
		query = query.Where("record_date <= ?", *filter.EndDate)
	}
	if filter.Verified != nil {
		query = query.Where("verified = ?", *filter.Verified)
	}

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	result := query.Order("record_date DESC").Limit(limit).Offset(offset).Find(&records)
	if result.Error != nil {
		log.Printf("GetByFilter: database error: %v", result.Error)
		return nil, ErrFailedToGetFarmRecordsByFilter
	}
	return records, nil
}

// GetVerifiedByUserID retrieves only verified farm records for a user
func (r *farmRecordRepository) GetVerifiedByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.FarmRecord, error) {
	var records []*models.FarmRecord
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND verified = ?", userID, true).
		Order("record_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&records)

	if result.Error != nil {
		log.Printf("GetVerifiedByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetVerifiedFarmRecords
	}
	return records, nil
}

// CountByUserID counts total farm records for a user
func (r *farmRecordRepository) CountByUserID(ctx context.Context, userID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.FarmRecord{}).
		Where("user_id = ?", userID).
		Count(&count)

	if result.Error != nil {
		log.Printf("CountByUserID: database error: %v", result.Error)
		return 0, ErrFailedToCountFarmRecords
	}
	return count, nil
}

// CountVerifiedByUserID counts verified farm records for a user
func (r *farmRecordRepository) CountVerifiedByUserID(ctx context.Context, userID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.FarmRecord{}).
		Where("user_id = ? AND verified = ?", userID, true).
		Count(&count)

	if result.Error != nil {
		log.Printf("CountVerifiedByUserID: database error: %v", result.Error)
		return 0, ErrFailedToCountVerifiedRecords
	}
	return count, nil
}

// GetUniqueCropTypes retrieves distinct crop types for a user
func (r *farmRecordRepository) GetUniqueCropTypes(ctx context.Context, userID string) ([]string, error) {
	var cropTypes []string
	result := r.db.WithContext(ctx).
		Model(&models.FarmRecord{}).
		Where("user_id = ? AND crop_type IS NOT NULL", userID).
		Distinct("crop_type").
		Pluck("crop_type", &cropTypes)

	if result.Error != nil {
		log.Printf("GetUniqueCropTypes: database error: %v", result.Error)
		return nil, ErrFailedToGetUniqueCropTypes
	}
	return cropTypes, nil
}

// GetSalesSummary retrieves total sales amount for a user within a date range
func (r *farmRecordRepository) GetSalesSummary(ctx context.Context, userID string, startDate, endDate time.Time) (int64, error) {
	var totalSales int64
	result := r.db.WithContext(ctx).
		Model(&models.FarmRecord{}).
		Where("user_id = ? AND record_type = ? AND record_date BETWEEN ? AND ?",
			userID, models.FarmRecordTypeSale, startDate, endDate).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalSales)

	if result.Error != nil {
		log.Printf("GetSalesSummary: database error: %v", result.Error)
		return 0, ErrFailedToGetSalesSummary
	}
	return totalSales, nil
}

// GetExpensesSummary retrieves total expenses for a user within a date range
func (r *farmRecordRepository) GetExpensesSummary(ctx context.Context, userID string, startDate, endDate time.Time) (int64, error) {
	var totalExpenses int64
	result := r.db.WithContext(ctx).
		Model(&models.FarmRecord{}).
		Where("user_id = ? AND record_type IN ? AND record_date BETWEEN ? AND ?",
			userID, []string{models.FarmRecordTypeExpense, models.FarmRecordTypeInput}, startDate, endDate).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalExpenses)

	if result.Error != nil {
		log.Printf("GetExpensesSummary: database error: %v", result.Error)
		return 0, ErrFailedToGetExpensesSummary
	}
	return totalExpenses, nil
}

// GetSummaryByUser retrieves comprehensive farm record statistics for a user
func (r *farmRecordRepository) GetSummaryByUser(ctx context.Context, userID string, startDate, endDate time.Time) (*models.FarmRecordSummary, error) {
	summary := &models.FarmRecordSummary{
		UserID:        userID,
		RecordsByType: make(map[string]int64),
		MonthlySales:  make(map[string]int64),
		DateRange: models.DateRange{
			Start: startDate,
			End:   endDate,
		},
	}

	// Get total and verified counts
	var counts struct {
		Total    int64 `gorm:"column:total"`
		Verified int64 `gorm:"column:verified"`
	}
	r.db.WithContext(ctx).
		Model(&models.FarmRecord{}).
		Where("user_id = ? AND record_date BETWEEN ? AND ?", userID, startDate, endDate).
		Select("COUNT(*) as total, SUM(CASE WHEN verified THEN 1 ELSE 0 END) as verified").
		Scan(&counts)

	summary.TotalRecords = counts.Total
	summary.VerifiedRecords = counts.Verified

	// Get sales and expenses totals
	var totals struct {
		TotalSales    int64 `gorm:"column:total_sales"`
		TotalExpenses int64 `gorm:"column:total_expenses"`
		TotalInputs   int64 `gorm:"column:total_inputs"`
	}
	r.db.WithContext(ctx).Raw(`
		SELECT
			COALESCE(SUM(CASE WHEN record_type = 'sale' THEN amount ELSE 0 END), 0) as total_sales,
			COALESCE(SUM(CASE WHEN record_type = 'expense' THEN amount ELSE 0 END), 0) as total_expenses,
			COALESCE(SUM(CASE WHEN record_type = 'input' THEN amount ELSE 0 END), 0) as total_inputs
		FROM farm_records
		WHERE user_id = ? AND record_date BETWEEN ? AND ? AND deleted_at IS NULL
	`, userID, startDate, endDate).Scan(&totals)

	summary.TotalSalesAmount = totals.TotalSales
	summary.TotalExpenses = totals.TotalExpenses
	summary.TotalInputCosts = totals.TotalInputs

	// Get unique crop types
	cropTypes, _ := r.GetUniqueCropTypes(ctx, userID)
	summary.CropTypes = cropTypes

	// Get records by type
	var typeBreakdown []struct {
		RecordType string `gorm:"column:record_type"`
		Count      int64  `gorm:"column:count"`
	}
	r.db.WithContext(ctx).
		Model(&models.FarmRecord{}).
		Where("user_id = ? AND record_date BETWEEN ? AND ?", userID, startDate, endDate).
		Select("record_type, COUNT(*) as count").
		Group("record_type").
		Scan(&typeBreakdown)

	for _, tb := range typeBreakdown {
		summary.RecordsByType[tb.RecordType] = tb.Count
	}

	// Get monthly sales breakdown
	var monthlySales []struct {
		Month string `gorm:"column:month"`
		Total int64  `gorm:"column:total"`
	}
	r.db.WithContext(ctx).Raw(`
		SELECT
			TO_CHAR(record_date, 'YYYY-MM') as month,
			COALESCE(SUM(amount), 0) as total
		FROM farm_records
		WHERE user_id = ? AND record_type = 'sale' AND record_date BETWEEN ? AND ? AND deleted_at IS NULL
		GROUP BY TO_CHAR(record_date, 'YYYY-MM')
		ORDER BY month ASC
	`, userID, startDate, endDate).Scan(&monthlySales)

	for _, ms := range monthlySales {
		summary.MonthlySales[ms.Month] = ms.Total
	}

	return summary, nil
}

// GetByDocumentID retrieves farm records linked to a specific document
func (r *farmRecordRepository) GetByDocumentID(ctx context.Context, documentID string) ([]*models.FarmRecord, error) {
	var records []*models.FarmRecord
	result := r.db.WithContext(ctx).
		Where("document_id = ?", documentID).
		Order("record_date ASC").
		Find(&records)

	if result.Error != nil {
		log.Printf("GetByDocumentID: database error: %v", result.Error)
		return nil, ErrFailedToGetFarmRecordsByDoc
	}
	return records, nil
}

// GetLatestByUserID retrieves the most recent farm records for a user
func (r *farmRecordRepository) GetLatestByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.FarmRecord, error) {
	var records []*models.FarmRecord
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("record_date DESC, created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&records)

	if result.Error != nil {
		log.Printf("GetLatestByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetLatestFarmRecords
	}
	return records, nil
}

// HasRecentActivity checks if user has farm activity within specified days
func (r *farmRecordRepository) HasRecentActivity(ctx context.Context, userID string, daysBack int) (bool, error) {
	var count int64
	cutoffDate := time.Now().AddDate(0, 0, -daysBack)

	result := r.db.WithContext(ctx).
		Model(&models.FarmRecord{}).
		Where("user_id = ? AND record_date >= ?", userID, cutoffDate).
		Count(&count)

	if result.Error != nil {
		log.Printf("HasRecentActivity: database error: %v", result.Error)
		return false, ErrFailedToCheckRecentActivity
	}
	return count > 0, nil
}

// --- Update operations ---

// Update updates a farm record
func (r *farmRecordRepository) Update(ctx context.Context, record *models.FarmRecord) error {
	result := r.db.WithContext(ctx).
		Model(record).
		Where("id = ?", record.ID).
		Updates(map[string]interface{}{
			"record_type": record.RecordType,
			"record_date": record.RecordDate,
			"crop_type":   record.CropType,
			"quantity":    record.Quantity,
			"unit":        record.Unit,
			"amount":      record.Amount,
			"description": record.Description,
			"document_id": record.DocumentID,
			"verified":    record.Verified,
			"metadata":    record.Metadata,
			"updated_at":  time.Now(),
		})

	if result.RowsAffected == 0 {
		return ErrFarmRecordNotFound
	}
	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateFarmRecord
	}
	return nil
}

// Restore restores a farm record by ID.
func (r *farmRecordRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.FarmRecord{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrFarmRecordNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreFarmRecord
	}
	return nil
}

// --- Delete operations ---

// Delete soft deletes a farm record
func (r *farmRecordRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.FarmRecord{})

	if result.RowsAffected == 0 {
		return ErrFarmRecordNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteFarmRecord
	}
	return nil
}
