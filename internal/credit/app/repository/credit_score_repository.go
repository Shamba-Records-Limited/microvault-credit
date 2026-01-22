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

// Common error for CreditScoreRepository
var (
	ErrCreditScoreNotFound                  = errors.New("no credit score found for user")
	ErrCreditScoreExpired                   = errors.New("credit score expired")
	ErrInvalidCreditScore                   = errors.New("invalid credit score")
	ErrFailedToCreateCreditScore            = errors.New("failed to create credit score")
	ErrFailedToCreateBatchCreditScore       = errors.New("failed to batch create credit scores")
	ErrFailedToGetLatestCreditScore         = errors.New("failed to get latest credit score")
	ErrFailedToGetCreditScoreHistory        = errors.New("failed to get credit score history")
	ErrFailedToCheckExpiration              = errors.New("failed to check credit score expiration")
	ErrFailedToGetUsersNeedingRecalculation = errors.New("failed to get users needing recalculation")
	ErrFailedToUpdateCreditScore            = errors.New("failed to update credit score")
	ErrFailedToRestoreCreditScore           = errors.New("failed to restore credit score")
	ErrFailedToDeleteCreditScore            = errors.New("failed to delete credit score")
	ErrFailedToDeleteCreditScoresByUserID   = errors.New("failed to delete credit scores by user ID")
)

// CreditScoreRepository defines the interface for credit score data access
type CreditScoreRepository interface {
	// Create operations
	Create(ctx context.Context, score *models.CreditScore) error
	BatchCreate(ctx context.Context, scores []*models.CreditScore) error

	// Read operations
	GetLatestByUserID(ctx context.Context, userID string) (*models.CreditScore, error)
	GetHistoryByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.CreditScore, error)
	GetUsersNeedingRecalculation(ctx context.Context, offset, limit int) ([]string, error)

	// Update operation
	Update(ctx context.Context, score *models.CreditScore) error
	Restore(ctx context.Context, id string) error

	// Delete operation
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error
}

// CreditScoreRepository represents a repository for managing credit scores.
type creditScoreRepository struct {
	db *gorm.DB
}

// NewCreditScoreRepository creates a new instance of CreditScoreRepository.
func NewCreditScoreRepository(db *gorm.DB) (CreditScoreRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &creditScoreRepository{db: db}, nil
}

// --- Create operations ---

// Create creates a new credit score record.
func (r *creditScoreRepository) Create(ctx context.Context, score *models.CreditScore) error {
	result := r.db.WithContext(ctx).Create(score)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateCreditScore
	}
	return nil
}

// BatchCreate creates multiple credit score records in a single transaction.
func (r *creditScoreRepository) BatchCreate(ctx context.Context, scores []*models.CreditScore) error {
	if len(scores) == 0 {
		return nil
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(scores).Error
	})
	if err != nil {
		if errors.Is(err, ErrInvalidCreditScore) {
			return ErrInvalidCreditScore
		}
		log.Printf("CreateBatch: database error: %v", err)
		return ErrFailedToCreateBatchCreditScore
	}
	return nil
}

// --- Read operations ---

// GetLatestByUserID retrieves the latest credit score for a user.
func (r *creditScoreRepository) GetLatestByUserID(ctx context.Context, userID string) (*models.CreditScore, error) {
	var score models.CreditScore
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("calculated_at DESC").
		First(&score)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrCreditScoreNotFound
	}
	if result.Error != nil {
		log.Printf("GetLatestByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetLatestCreditScore
	}
	return &score, nil
}

// GetHistoryByUserID retrieves the history of credit scores for a user.
func (r *creditScoreRepository) GetHistoryByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.CreditScore, error) {
	var scores []*models.CreditScore
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("calculated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&scores)
	if result.Error != nil {
		log.Printf("GetHistoryByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditScoreHistory
	}
	return scores, nil
}

// IsExpired checks if a credit score has expired.
func (r *creditScoreRepository) IsExpired(ctx context.Context, id string) (bool, error) {
	var score models.CreditScore
	result := r.db.WithContext(ctx).
		Select("expires_at").
		Where("id = ?", id).
		First(&score)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, ErrCreditScoreNotFound
	}

	if result.Error != nil {
		log.Printf("IsExpired: database error: %v", result.Error)
		return false, ErrFailedToCheckExpiration
	}

	if score.ExpiresAt == nil {
		return false, nil
	}

	return score.IsExpired(), nil
}

// GetUsersNeedingRecalculation retrieves a list of user IDs that need their credit scores recalculated.
func (r *creditScoreRepository) GetUsersNeedingRecalculation(ctx context.Context, limit, offset int) ([]string, error) {
	var userIDs []string

	sevenDaysAgo := time.Now().AddDate(0, 0, -7)

	// Find users with no score, expired score, or score older than 7 days
	result := r.db.WithContext(ctx).Raw(`
        SELECT DISTINCT u.id
        FROM users u
        LEFT JOIN credit_scores cs ON u.id = cs.user_id
        WHERE u.status = 'active'
          AND (
            cs.id IS NULL
            OR cs.expires_at < NOW()
            OR cs.calculated_at < ?
          )

        LIMIT ?
        OFFSET ?
    `, sevenDaysAgo, limit, offset).Scan(&userIDs)

	if result.Error != nil {
		log.Printf("GetUsersNeedingRecalculation: database error: %v", result.Error)
		return nil, ErrFailedToGetUsersNeedingRecalculation
	}

	return userIDs, nil
}

// --- Update operations ---

func (r *creditScoreRepository) Update(ctx context.Context, score *models.CreditScore) error {
	if score.IsExpired() {
		return ErrCreditScoreExpired
	}

	// To:do
	// - Implement score calculation

	result := r.db.WithContext(ctx).
		Model(score).
		Updates(map[string]interface{}{
			"score":                 score.Score,
			"score_version":         score.ScoreVersion,
			"total_loans":           score.TotalLoans,
			"successful_repayments": score.SuccessfulRepayments,
			"defaults":              score.Defaults,
			"current_outstanding":   score.CurrentOutstanding,
			"days_overdue":          score.DaysOverdue,
			"max_loan_amount":       score.MaxLoanAmount,
			"max_concurrent_loans":  score.MaxConcurrentLoans,
			"calculated_at":         score.CalculatedAt,
			"updated_at":            time.Now(),
		})

	if result.RowsAffected == 0 {
		return ErrCreditScoreNotFound
	}
	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateCreditScore
	}
	return nil
}

// Restore restores a credit score by ID.
func (r *creditScoreRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.CreditScore{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrCreditScoreNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreCreditScore
	}
	return nil
}

// --- Delete operations ---

// Delete deletes a credit score.
func (r *creditScoreRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&models.CreditScore{}, "id = ?", id)
	if result.RowsAffected == 0 {
		return ErrCreditScoreNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteCreditScore
	}
	return nil
}

// DeleteByUserID deletes all credit scores for a user.
func (r *creditScoreRepository) DeleteByUserID(ctx context.Context, userID string) error {
	result := r.db.WithContext(ctx).Delete(&models.CreditScore{}, "user_id = ?", userID)
	if result.Error != nil {
		log.Printf("DeleteByUserID: database error: %v", result.Error)
		return ErrFailedToDeleteCreditScoresByUserID
	}
	return nil
}
