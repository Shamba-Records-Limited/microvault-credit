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

// Common errors for LoanRepository
var (
	ErrLoanNotFound                   = errors.New("loan not found")
	ErrFailedToCreateLoan             = errors.New("failed to create loan")
	ErrFailedToGetLoan                = errors.New("failed to get loan")
	ErrFailedToGetLoansByUserID       = errors.New("failed to get loans by user ID")
	ErrFailedToGetActiveLoans         = errors.New("failed to get active loans")
	ErrFailedToGetActiveLoansByStatus = errors.New("failed to get active loans by status")
	ErrFailedToUpdateLoan             = errors.New("failed to update loan")
	ErrFailedToRestoreLoan            = errors.New("failed to restore loan")
	ErrFailedToDeleteLoan             = errors.New("failed to delete loan")
	ErrFailedToDeleteLoansByUserID    = errors.New("failed to delete loans by user ID")
	ErrFailedToGetLoanBySequenceID   = errors.New("failed to get loan by sequence ID")
	ErrFailedToGetLoansByDisbStatus  = errors.New("failed to get loans by disbursement status")
	ErrFailedToGetActiveByProvider   = errors.New("failed to get active loans by ramp provider")
)

// LoanRepository defines the interface for loanRespository data access.
type LoanRepository interface {
	// Create operations
	Create(ctx context.Context, loan *models.Loan) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.Loan, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.Loan, error)
	GetActiveLoans(ctx context.Context, limit, offset int) ([]*models.Loan, error)
	GetActiveLoansByStatus(ctx context.Context, status string, limit, offset int) ([]*models.Loan, error)
	GetBySequenceID(ctx context.Context, sequenceID string) (*models.Loan, error)
	GetByDisbursementStatus(ctx context.Context, status string, limit int) ([]*models.Loan, error)

	// GetActiveByProvider returns loans where ramp_provider matches the given
	// provider and disbursement_status is in a non-terminal state. Used by
	// provider-specific pollers to enumerate work.
	GetActiveByProvider(ctx context.Context, provider string, limit int) ([]*models.Loan, error)

	// GetActiveByUserAndProvider scopes the same query to a single user.
	// Used as the dedupe gate: a non-empty result means the user already has
	// an in-flight off-ramp for that provider and a new request should fail.
	GetActiveByUserAndProvider(ctx context.Context, userID, provider string) ([]*models.Loan, error)

	// Update operations
	Update(ctx context.Context, loan *models.Loan) error
	Restore(ctx context.Context, id string) error

	// Delete operations
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error
}

// loanRepository represents a repository for managing loans.
type loanRepository struct {
	db *gorm.DB
}

// NewLoanRepository creates a new instance of LoanRepository.
func NewLoanRepository(db *gorm.DB) (LoanRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &loanRepository{db: db}, nil
}

// --- Create Operations ---

// Create creates a new loan record.
func (r *loanRepository) Create(ctx context.Context, loan *models.Loan) error {
	result := r.db.WithContext(ctx).Create(loan)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateLoan
	}
	return nil
}

// --- Read Operations ---

// GetByID retrieves a loan record by its ID.
func (r *loanRepository) GetByID(ctx context.Context, id string) (*models.Loan, error) {
	var loan models.Loan
	result := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&loan)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetLoan
	}
	return &loan, nil
}

// GetByUserID retrieves all loan records for a given user ID.
func (r *loanRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.Loan, error) {
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetLoansByUserID
	}
	return loans, nil
}

// GetActiveLoans returns a list of active loans.
func (r *loanRepository) GetActiveLoans(ctx context.Context, limit, offset int) ([]*models.Loan, error) {
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Where("status IN ? AND deleted_at IS NULL",
			[]string{models.LoanStatusApproved, models.LoanStatusDisbursed}).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetActiveLoans: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveLoans
	}
	return loans, nil
}

// GetActiveLoansByStatus returns a list of active loans by status.
func (r *loanRepository) GetActiveLoansByStatus(ctx context.Context, status string, limit, offset int) ([]*models.Loan, error) {
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Where("status IN ? AND deleted_at IS NULL", status).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetActiveLoansByStatus: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveLoansByStatus
	}
	return loans, nil
}

// GetBySequenceID retrieves a loan by its YellowCard ramp sequence ID.
func (r *loanRepository) GetBySequenceID(ctx context.Context, sequenceID string) (*models.Loan, error) {
	var loan models.Loan
	result := r.db.WithContext(ctx).
		Preload("User").
		Where("ramp_sequence_id = ? AND deleted_at IS NULL", sequenceID).
		First(&loan)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("GetBySequenceID: database error: %v", result.Error)
		return nil, ErrFailedToGetLoanBySequenceID
	}
	return &loan, nil
}

// GetByDisbursementStatus retrieves loans by their disbursement status.
func (r *loanRepository) GetByDisbursementStatus(ctx context.Context, status string, limit int) ([]*models.Loan, error) {
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Preload("User").
		Where("disbursement_status = ? AND deleted_at IS NULL", status).
		Order("created_at ASC").
		Limit(limit).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetByDisbursementStatus: database error: %v", result.Error)
		return nil, ErrFailedToGetLoansByDisbStatus
	}
	return loans, nil
}

// --- Update Operations ---

// Update updates a loan record.
func (r *loanRepository) Update(ctx context.Context, loan *models.Loan) error {
	result := r.db.WithContext(ctx).
		Model(loan).
		Where("id = ? AND deleted_at IS NULL", loan.ID).
		Updates(map[string]interface{}{
			"status":              loan.Status,
			"approved_at":         loan.ApprovedAt,
			"approved_by":         loan.ApprovedBy,
			"disbursed_at":        loan.DisbursedAt,
			"repaid_at":           loan.RepaidAt,
			"defaulted_at":        loan.DefaultedAt,
			"vault_tx_hash":       loan.VaultTxHash,
			"vault_tx_status":     loan.VaultTxStatus,
			"vault_repay_tx_hash": loan.VaultRepayTxHash,
			"ramp_provider":       loan.RampProvider,
			"ramp_request_id":     loan.RampRequestID,
			"ramp_fiat_amount":    loan.RampFiatAmount,
			"ramp_fiat_currency":  loan.RampFiatCurr,
			"momo_provider":       loan.MomoProvider,
			"momo_transaction_id":  loan.MomoTxID,
			"momo_status":          loan.MomoStatus,
			"settlement_method":    loan.SettlementMethod,
			"disbursement_status":  loan.DisbursementStatus,
			"ramp_sequence_id":     loan.RampSequenceID,
			"origination_fee":          loan.OriginationFee,
			"origination_fee_bps":     loan.OriginationFeeBps,
			"total_amount":            loan.TotalAmount,
			"disbursement_rate_bps":   loan.DisbursementRateBps,
			"disbursement_amount_kes": loan.DisbursementAmtKES,
			"repayment_amount_kes":    loan.RepaymentAmtKES,
			"conversion_spread_bps":   loan.ConversionSpreadBps,
			"borrow_index":            loan.BorrowIndex,
			"ramp_fee_usd":            loan.RampFeeUSD,
			"ramp_fee_local":          loan.RampFeeLocal,
			"updated_at":              time.Now(),
		})
	if result.RowsAffected == 0 {
		return ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateLoan
	}
	return nil
}

// Restore restores a loan by ID.
func (r *loanRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.Loan{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreLoan
	}
	return nil
}

// --- Delete operations ---

// Delete deletes a loan by ID.
func (r *loanRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Loan{})
	if result.RowsAffected == 0 {
		return ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteLoan
	}
	return nil
}

// DeleteByUserID deletes a loan by user ID.
func (r *loanRepository) DeleteByUserID(ctx context.Context, userID string) error {
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&models.Loan{})
	if result.Error != nil {
		log.Printf("DeleteByUserID: database error: %v", result.Error)
		return ErrFailedToDeleteLoansByUserID
	}
	return nil
}
