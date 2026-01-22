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

// Common Errors for RepaymentRepository
var (
	ErrRepaymentNotFound              = errors.New("repayment not found")
	ErrFailedToCreateRepayment        = errors.New("failed to create repayment")
	ErrFailedToCreateRepayments       = errors.New("failed to create repayments")
	ErrFailedToGetRepayment           = errors.New("failed to get repayment")
	ErrFailedToGetRepayments          = errors.New("failed to get repayments")
	ErrFailedToGetRepaymentsByUser    = errors.New("failed to get repayments by user")
	ErrFailedToGetRepaymentsByLoan    = errors.New("failed to get repayments by loan")
	ErrFailedToGetOverdueRepayments   = errors.New("failed to get overdue repayments")
	ErrFailedToGetTotalAmountDue      = errors.New("failed to get total amount due")
	ErrFailedToGetUpcomingRepayments  = errors.New("failed to get upcoming repayments")
	ErrFailedToUpdateRepayment        = errors.New("failed to update repayment")
	ErrFailedToRestoreRepayment       = errors.New("failed to restore repayment")
	ErrFailedToDeleteRepayment        = errors.New("failed to delete repayment")
	ErrFailedToDeleteRepaymentsByLoan = errors.New("failed to delete repayments by loan")
	ErrFailedToDeleteRepaymentsByUser = errors.New("failed to delete repayments by user")
)

// RepaymentRepository defines the interface for repayments data access.
type RepaymentRepository interface {
	// Create operations
	Create(ctx context.Context, repayment *models.Repayment) error
	BatchCreate(ctx context.Context, repayments []*models.Repayment) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.Repayment, error)
	GetByUserID(ctx context.Context, userID string, offset, limit int) ([]*models.Repayment, error)
	GetByLoanID(ctx context.Context, loanID string, offset, limit int) ([]*models.Repayment, error)
	GetAll(ctx context.Context, offset, limit int) ([]*models.Repayment, error)
	GetOverdue(ctx context.Context, asOfDate time.Time, offset, limit int) ([]*models.Repayment, error)
	GetTotalAmountDue(ctx context.Context, loanID string) (int64, error)
	GetUpcoming(ctx context.Context, from, to time.Time, offset, limit int) ([]*models.Repayment, error)

	// Update operations
	Update(ctx context.Context, repayment *models.Repayment) error
	Restore(ctx context.Context, id string) error

	// Delete operations
	Delete(ctx context.Context, id string) error
	DeleteByLoanID(ctx context.Context, loanID string) error
	DeleteByUserID(ctx context.Context, userID string) error
}

// RepaymentRepository represents a repository for managing repayments.
type repaymentRepository struct {
	db *gorm.DB
}

// NewRepaymentRepository creates a new instance of RepaymentRepository.
func NewRepaymentRepository(db *gorm.DB) (RepaymentRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &repaymentRepository{db: db}, nil
}

// --- Create operations ---

// Create creates a new repayment record.
func (r *repaymentRepository) Create(ctx context.Context, repayment *models.Repayment) error {
	result := r.db.WithContext(ctx).Create(repayment)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateRepayment
	}
	return nil
}

// BatchCreate creates multiple repayment records in a single transaction.
// This is typically used when generating a repayment schedule for a disbursed loan.
func (r *repaymentRepository) BatchCreate(ctx context.Context, repayments []*models.Repayment) error {
	if len(repayments) == 0 {
		return nil
	}

	result := r.db.WithContext(ctx).Create(&repayments)
	if result.Error != nil {
		log.Printf("BatchCreate: database error: %v", result.Error)
		return ErrFailedToCreateRepayments
	}

	log.Printf("BatchCreate: successfully created %d repayments", len(repayments))
	return nil
}

// --- Read operations ---

// GetByID retrieves a repayment by its ID.
func (r *repaymentRepository) GetByID(ctx context.Context, id string) (*models.Repayment, error) {
	var repayment models.Repayment
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&repayment)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrRepaymentNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetRepayment
	}
	return &repayment, nil
}

// GetByLoanID retrieves all repayments for a given loan ID.
func (r *repaymentRepository) GetByLoanID(ctx context.Context, loanID string, offset, limit int) ([]*models.Repayment, error) {
	var repayments []*models.Repayment
	result := r.db.WithContext(ctx).
		Where("loan_id = ?", loanID).
		Order("installment_number ASC").
		Offset(offset).
		Limit(limit).
		Find(&repayments)
	if result.Error != nil {
		log.Printf("GetByLoanID: database error: %v", result.Error)
		return nil, ErrFailedToGetRepaymentsByLoan
	}
	return repayments, nil
}

// GetByUserID retrieves all repayments for a given user ID.
func (r *repaymentRepository) GetByUserID(ctx context.Context, userID string, offset, limit int) ([]*models.Repayment, error) {
	var repayments []*models.Repayment
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id ASC").
		Offset(offset).
		Limit(limit).
		Find(&repayments)
	if result.Error != nil {
		log.Printf("GetByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetRepaymentsByUser
	}
	return repayments, nil
}

// GetAll retrieves all repayments.
func (r *repaymentRepository) GetAll(ctx context.Context, offset, limit int) ([]*models.Repayment, error) {
	var repayments []*models.Repayment
	result := r.db.WithContext(ctx).
		Order("id ASC").
		Offset(offset).
		Limit(limit).
		Find(&repayments)
	if result.Error != nil {
		log.Printf("GetAll: database error: %v", result.Error)
		return nil, ErrFailedToGetRepayments
	}
	return repayments, nil
}

// GetOverdue retrieves all overdue repayments.
func (r *repaymentRepository) GetOverdue(ctx context.Context, asOfDate time.Time, offset, limit int) ([]*models.Repayment, error) {
	var repayments []*models.Repayment
	result := r.db.WithContext(ctx).
		Where("status IN ? AND due_date < ?",
			[]string{models.RepaymentStatusPending, models.RepaymentStatusPartial},
			asOfDate).
		Order("due_date ASC").
		Offset(offset).
		Limit(limit).
		Find(&repayments)
	if result.Error != nil {
		log.Printf("GetOverdue: database error: %v", result.Error)
		return nil, ErrFailedToGetOverdueRepayments
	}
	return repayments, nil
}

// GetTotalAmountDue returns the total amount due for a loan.
func (r *repaymentRepository) GetTotalAmountDue(ctx context.Context, loanID string) (int64, error) {
	var totalDue int64
	result := r.db.WithContext(ctx).
		Model(&models.Repayment{}).
		Where("loan_id = ? AND status IN ?", loanID,
			[]string{models.RepaymentStatusPending, models.RepaymentStatusOverdue, models.RepaymentStatusPartial}).
		Select("COALESCE(SUM(amount_due + late_fee - amount_paid), 0)").
		Scan(&totalDue)
	if result.Error != nil {
		log.Printf("GetTotalAmountDue: database error: %v", result.Error)
		return 0, ErrFailedToGetTotalAmountDue
	}
	return totalDue, nil
}

// GetUpcoming retrieves all upcoming repayments within a given date range.
func (r *repaymentRepository) GetUpcoming(ctx context.Context, startDate, endDate time.Time, offset, limit int) ([]*models.Repayment, error) {
	var repayments []*models.Repayment
	result := r.db.WithContext(ctx).
		Where("status = ? AND due_date BETWEEN ? AND ?",
			models.RepaymentStatusPending, startDate, endDate).
		Order("due_date ASC").
		Offset(offset).
		Limit(limit).
		Find(&repayments)
	if result.Error != nil {
		log.Printf("GetUpcoming: database error: %v", result.Error)
		return nil, ErrFailedToGetUpcomingRepayments
	}
	return repayments, nil
}

// --- Update operations ---

// Update updates a repayment record.
func (r *repaymentRepository) Update(ctx context.Context, repayment *models.Repayment) error {
	result := r.db.WithContext(ctx).
		Model(repayment).
		Where("id = ?", repayment.ID).
		Updates(map[string]interface{}{
			"amount_due":     repayment.AmountDue,
			"amount_paid":    repayment.AmountPaid,
			"due_date":       repayment.DueDate,
			"paid_at":        repayment.PaidAt,
			"status":         repayment.Status,
			"late_fee":       repayment.LateFee,
			"payment_method": repayment.PaymentMethod,
			"transaction_id": repayment.TransactionID,
			"updated_at":     time.Now(),
		})
	if result.RowsAffected == 0 {
		return ErrRepaymentNotFound
	}
	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateRepayment
	}
	return nil
}

// Restore restores a deleted repayment.
func (r *repaymentRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.Repayment{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrRepaymentNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreRepayment
	}
	return nil
}

// --- Delete operations ---

// Delete deletes a repayment by ID.
func (r *repaymentRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Repayment{})
	if result.RowsAffected == 0 {
		return ErrRepaymentNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteRepayment
	}
	return nil
}

// DeleteByLoanID deletes repayments by loan ID.
func (r *repaymentRepository) DeleteByLoanID(ctx context.Context, loanID string) error {
	result := r.db.WithContext(ctx).
		Where("loan_id = ?", loanID).
		Delete(&models.Repayment{})
	if result.Error != nil {
		log.Printf("DeleteByLoanID: database error: %v", result.Error)
		return ErrFailedToDeleteRepaymentsByLoan
	}
	return nil
}

// DeleteByUserID deletes repayments by user ID.
func (r *repaymentRepository) DeleteByUserID(ctx context.Context, userID string) error {
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&models.Repayment{})
	if result.Error != nil {
		log.Printf("DeleteByUserID: database error: %v", result.Error)
		return ErrFailedToDeleteRepaymentsByUser
	}
	return nil
}
