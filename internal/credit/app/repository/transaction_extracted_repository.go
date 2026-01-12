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

// Transaction extracted repository errors
var (
	ErrTransactionNotFound               = errors.New("transaction not found")
	ErrFailedToCreateTransaction         = errors.New("failed to create transaction")
	ErrFailedToCreateTransactions        = errors.New("failed to create transactions")
	ErrFailedToGetTransaction            = errors.New("failed to get transaction")
	ErrFailedToGetTransactions           = errors.New("failed to get transactions")
	ErrFailedToGetIncomeTransactions     = errors.New("failed to get income transactions")
	ErrFailedToGetExpenseTransactions    = errors.New("failed to get expense transactions")
	ErrFailedToGetTransactionsByDocument = errors.New("failed to get transactions by document")
	ErrFailedToGetTransactionsByCategory = errors.New("failed to get transactions by category")
	ErrFailedToGetTransactionsBySource   = errors.New("failed to get transactions by source")
	ErrFailedToGetTransactionsByType     = errors.New("failed to get transactions by type")
	ErrFailedToCountTransactions         = errors.New("failed to count transactions")
	ErrFailedToGetTransactionSummary     = errors.New("failed to get transaction summary")
	ErrFailedToRestoreTransaction        = errors.New("failed to restore transaction")
	ErrFailedToDeleteTransaction         = errors.New("failed to delete transaction")
)

// TransactionExtractedRepository defines the interface for transaction extracted data operations
type TransactionExtractedRepository interface {
	// --- Create operations ---
	Create(ctx context.Context, transaction *models.TransactionExtracted) error
	BatchCreate(ctx context.Context, transactions []*models.TransactionExtracted) error

	// --- Read operations ---
	GetByID(ctx context.Context, id string) (*models.TransactionExtracted, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.TransactionExtracted, error)
	GetByDocumentID(ctx context.Context, documentID string) ([]*models.TransactionExtracted, error)
	GetByDateRange(ctx context.Context, userID string, startDate, endDate time.Time, limit, offset int) ([]*models.TransactionExtracted, error)
	GetByCategory(ctx context.Context, userID string, category string, limit, offset int) ([]*models.TransactionExtracted, error)
	GetBySource(ctx context.Context, userID string, source string, limit, offset int) ([]*models.TransactionExtracted, error)
	GetByTransactionType(ctx context.Context, userID string, txType string, startDate, endDate time.Time, limit, offset int) ([]*models.TransactionExtracted, error)
	GetIncomeTransactions(ctx context.Context, userID string, startDate, endDate time.Time) ([]*models.TransactionExtracted, error)
	GetExpenseTransactions(ctx context.Context, userID string, startDate, endDate time.Time) ([]*models.TransactionExtracted, error)
	GetRecentTransactions(ctx context.Context, userID string, limit int) ([]*models.TransactionExtracted, error)
	GetByCounterparty(ctx context.Context, userID string, counterparty string, limit, offset int) ([]*models.TransactionExtracted, error)

	// --- Aggregation operations ---
	CountByUserID(ctx context.Context, userID string) (int64, error)
	CountBySource(ctx context.Context, userID string, source string) (int64, error)
	CountByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) (int64, error)
	GetSummary(ctx context.Context, userID string, startDate, endDate time.Time) (map[string]interface{}, error)

	// --- Update operations ---
	Restore(ctx context.Context, id string) error

	// --- Delete operations ---
	Delete(ctx context.Context, id string) error
	DeleteByDocumentID(ctx context.Context, documentID string) error
}

// transactionExtractedRepository represents a repository for managing transaction extracted data.
type transactionExtractedRepository struct {
	db *gorm.DB
}

// NewTransactionExtractedRepository creates a new instance of TransactionExtractedRepository.
func NewTransactionExtractedRepository(db *gorm.DB) (TransactionExtractedRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &transactionExtractedRepository{db: db}, nil
}

// --- Create operations ---

// Create creates a single transaction extracted record.
func (r *transactionExtractedRepository) Create(ctx context.Context, transaction *models.TransactionExtracted) error {
	if err := r.db.WithContext(ctx).Create(transaction).Error; err != nil {
		log.Printf("Create: database error: %v", err)
		return ErrFailedToCreateTransaction
	}
	return nil
}

// BatchCreate creates multiple transaction extracted records in batches.
func (r *transactionExtractedRepository) BatchCreate(ctx context.Context, transactions []*models.TransactionExtracted) error {
	if len(transactions) == 0 {
		return nil
	}

	if err := r.db.WithContext(ctx).Create(transactions).Error; err != nil {
		log.Printf("BatchCreate: database error: %v", err)
		return ErrFailedToCreateTransactions
	}
	return nil
}

// --- Read operations ---

// GetByID retrieves a single transaction by ID
func (r *transactionExtractedRepository) GetByID(ctx context.Context, id string) (*models.TransactionExtracted, error) {
	var transaction models.TransactionExtracted
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&transaction)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrTransactionNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetTransaction
	}
	return &transaction, nil
}

// GetByUserID retrieves all transactions for a user with pagination
func (r *transactionExtractedRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.TransactionExtracted, error) {
	var transactions []*models.TransactionExtracted
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("transaction_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions)

	if result.Error != nil {
		log.Printf("GetByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetTransactions
	}
	return transactions, nil
}

// GetByDocumentID retrieves all transactions extracted from a specific document
func (r *transactionExtractedRepository) GetByDocumentID(ctx context.Context, documentID string) ([]*models.TransactionExtracted, error) {
	var transactions []*models.TransactionExtracted
	result := r.db.WithContext(ctx).
		Where("document_id = ?", documentID).
		Order("transaction_date ASC").
		Find(&transactions)

	if result.Error != nil {
		log.Printf("GetByDocumentID: database error: %v", result.Error)
		return nil, ErrFailedToGetTransactionsByDocument
	}
	return transactions, nil
}

// GetByDateRange retrieves transactions within a date range for a user
func (r *transactionExtractedRepository) GetByDateRange(ctx context.Context, userID string, startDate, endDate time.Time, limit, offset int) ([]*models.TransactionExtracted, error) {
	var transactions []*models.TransactionExtracted
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND transaction_date BETWEEN ? AND ?", userID, startDate, endDate).
		Order("transaction_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions)

	if result.Error != nil {
		log.Printf("GetByDateRange: database error: %v", result.Error)
		return nil, ErrFailedToGetTransactions
	}
	return transactions, nil
}

// GetByCategory retrieves transactions by category for a user
func (r *transactionExtractedRepository) GetByCategory(ctx context.Context, userID string, category string, limit, offset int) ([]*models.TransactionExtracted, error) {
	var transactions []*models.TransactionExtracted
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND category = ?", userID, category).
		Order("transaction_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions)

	if result.Error != nil {
		log.Printf("GetByCategory: database error: %v", result.Error)
		return nil, ErrFailedToGetTransactionsByCategory
	}
	return transactions, nil
}

// GetBySource retrieves transactions by source (mpesa, bank, etc.) for a user
func (r *transactionExtractedRepository) GetBySource(ctx context.Context, userID string, source string, limit, offset int) ([]*models.TransactionExtracted, error) {
	var transactions []*models.TransactionExtracted
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND source = ?", userID, source).
		Order("transaction_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions)

	if result.Error != nil {
		log.Printf("GetBySource: database error: %v", result.Error)
		return nil, ErrFailedToGetTransactionsBySource
	}
	return transactions, nil
}

// GetByTransactionType retrieves transactions by type within a date range
func (r *transactionExtractedRepository) GetByTransactionType(ctx context.Context, userID string, txType string, startDate, endDate time.Time, limit, offset int) ([]*models.TransactionExtracted, error) {
	var transactions []*models.TransactionExtracted
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND transaction_type = ? AND transaction_date BETWEEN ? AND ?",
			userID, txType, startDate, endDate).
		Order("transaction_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions)

	if result.Error != nil {
		log.Printf("GetByTransactionType: database error: %v", result.Error)
		return nil, ErrFailedToGetTransactionsByType
	}
	return transactions, nil
}

// GetIncomeTransactions retrieves income transactions for a user within a specified date range.
func (r *transactionExtractedRepository) GetIncomeTransactions(ctx context.Context, userID string, startDate, endDate time.Time) ([]*models.TransactionExtracted, error) {
	var transactions []*models.TransactionExtracted
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND transaction_date BETWEEN ? AND ? AND transaction_type IN ?",
			userID, startDate, endDate, []string{"receive", "deposit"}).
		Order("transaction_date ASC").
		Find(&transactions)

	if result.Error != nil {
		log.Printf("GetIncomeTransactions: database error: %v", result.Error)
		return nil, ErrFailedToGetIncomeTransactions
	}
	return transactions, nil
}

// GetExpenseTransactions retrieves expense transactions for a user within a specified date range.
func (r *transactionExtractedRepository) GetExpenseTransactions(ctx context.Context, userID string, startDate, endDate time.Time) ([]*models.TransactionExtracted, error) {
	var transactions []*models.TransactionExtracted
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND transaction_date BETWEEN ? AND ? AND transaction_type IN ?",
			userID, startDate, endDate, []string{"send", "withdraw"}).
		Order("transaction_date ASC").
		Find(&transactions)

	if result.Error != nil {
		log.Printf("GetExpenseTransactions: database error: %v", result.Error)
		return nil, ErrFailedToGetExpenseTransactions
	}
	return transactions, nil
}

// --- Aggregation operations ---

// CountByUserID counts total transactions for a user
func (r *transactionExtractedRepository) CountByUserID(ctx context.Context, userID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.TransactionExtracted{}).
		Where("user_id = ?", userID).
		Count(&count)

	if result.Error != nil {
		log.Printf("CountByUserID: database error: %v", result.Error)
		return 0, ErrFailedToCountTransactions
	}
	return count, nil
}

// CountBySource counts transactions by source for a user
func (r *transactionExtractedRepository) CountBySource(ctx context.Context, userID string, source string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.TransactionExtracted{}).
		Where("user_id = ? AND source = ?", userID, source).
		Count(&count)

	if result.Error != nil {
		log.Printf("CountBySource: database error: %v", result.Error)
		return 0, ErrFailedToCountTransactions
	}
	return count, nil
}

// CountByDateRange counts transactions within a date range for a user
func (r *transactionExtractedRepository) CountByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.TransactionExtracted{}).
		Where("user_id = ? AND transaction_date BETWEEN ? AND ?", userID, startDate, endDate).
		Count(&count)

	if result.Error != nil {
		log.Printf("CountByDateRange: database error: %v", result.Error)
		return 0, ErrFailedToCountTransactions
	}
	return count, nil
}

// GetSummary retrieves summary information for a user within a specified date range.
func (r *transactionExtractedRepository) GetSummary(ctx context.Context, userID string, startDate, endDate time.Time) (map[string]interface{}, error) {
	var result struct {
		TotalCount          int   `gorm:"column:total_count"`
		TotalIncome         int64 `gorm:"column:total_income"`
		TotalExpenses       int64 `gorm:"column:total_expenses"`
		MonthsWithActivity  int   `gorm:"column:months_with_activity"`
		UniqueIncomeSources int   `gorm:"column:unique_income_sources"`
	}

	err := r.db.WithContext(ctx).Raw(`
        SELECT
            COUNT(*) as total_count,
            SUM(CASE WHEN transaction_type IN ('receive', 'deposit') THEN amount ELSE 0 END) as total_income,
            SUM(CASE WHEN transaction_type IN ('send', 'withdraw') THEN amount ELSE 0 END) as total_expenses,
            COUNT(DISTINCT DATE_TRUNC('month', transaction_date)) as months_with_activity,
            COUNT(DISTINCT CASE WHEN transaction_type IN ('receive', 'deposit') THEN counterparty END) as unique_income_sources
        FROM transactions_extracted
        WHERE user_id = ? AND transaction_date BETWEEN ? AND ?
    `, userID, startDate, endDate).Scan(&result).Error

	if err != nil {
		log.Printf("GetSummary: database error: %v", err)
		return nil, ErrFailedToGetTransactionSummary
	}

	monthsActivity := result.MonthsWithActivity
	if monthsActivity == 0 {
		monthsActivity = 1
	}

	return map[string]interface{}{
		"total_count":           result.TotalCount,
		"total_income":          result.TotalIncome,
		"total_expenses":        result.TotalExpenses,
		"net_cashflow":          result.TotalIncome - result.TotalExpenses,
		"months_with_activity":  result.MonthsWithActivity,
		"unique_income_sources": result.UniqueIncomeSources,
		"avg_monthly_income":    result.TotalIncome / int64(monthsActivity),
		"avg_monthly_expenses":  result.TotalExpenses / int64(monthsActivity),
	}, nil
}

// GetRecentTransactions retrieves the most recent transactions for a user
func (r *transactionExtractedRepository) GetRecentTransactions(ctx context.Context, userID string, limit int) ([]*models.TransactionExtracted, error) {
	var transactions []*models.TransactionExtracted
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("transaction_date DESC, created_at DESC").
		Limit(limit).
		Find(&transactions)

	if result.Error != nil {
		log.Printf("GetRecentTransactions: database error: %v", result.Error)
		return nil, ErrFailedToGetTransactions
	}
	return transactions, nil
}

// GetByCounterparty retrieves transactions for a specific counterparty
func (r *transactionExtractedRepository) GetByCounterparty(ctx context.Context, userID string, counterparty string, limit, offset int) ([]*models.TransactionExtracted, error) {
	var transactions []*models.TransactionExtracted
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND counterparty = ?", userID, counterparty).
		Order("transaction_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions)

	if result.Error != nil {
		log.Printf("GetByCounterparty: database error: %v", result.Error)
		return nil, ErrFailedToGetTransactions
	}
	return transactions, nil
}

// --- Update operations ---

// Restore restores a transaction by ID.
func (r *transactionExtractedRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.TransactionExtracted{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrTransactionNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreTransaction
	}
	return nil
}

// --- Delete operations ---

// Delete deletes a transaction by ID
func (r *transactionExtractedRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.TransactionExtracted{})

	if result.RowsAffected == 0 {
		return ErrTransactionNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteTransaction
	}
	return nil
}

// DeleteByDocumentID deletes all transactions associated with a document
func (r *transactionExtractedRepository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	result := r.db.WithContext(ctx).
		Where("document_id = ?", documentID).
		Delete(&models.TransactionExtracted{})

	if result.Error != nil {
		log.Printf("DeleteByDocumentID: database error: %v", result.Error)
		return ErrFailedToDeleteTransaction
	}
	return nil
}
