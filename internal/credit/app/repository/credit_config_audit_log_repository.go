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

// Common errors for CreditConfigAuditLogRepository
var (
	ErrCreditConfigAuditLogNotFound                = errors.New("credit config audit log not found")
	ErrFailedToCreateCreditConfigAuditLog          = errors.New("failed to create credit config audit log")
	ErrFailedToCreateBatchCreditConfigAuditLogs    = errors.New("failed to create batch credit config audit logs")
	ErrFailedToGetCreditConfigAuditLog             = errors.New("failed to get credit config audit log")
	ErrFailedToGetCreditConfigAuditLogsByTable     = errors.New("failed to get credit config audit logs by table")
	ErrFailedToGetCreditConfigAuditLogsByConfigID  = errors.New("failed to get credit config audit logs by config ID")
	ErrFailedToGetCreditConfigAuditLogsByAction    = errors.New("failed to get credit config audit logs by action")
	ErrFailedToGetCreditConfigAuditLogsByDateRange = errors.New("failed to get credit config audit logs by date range")
	ErrFailedToGetAllCreditConfigAuditLogs         = errors.New("failed to get all credit config audit logs")
)

// CreditConfigAuditLogRepository defines the interface for credit config audit log data access
type CreditConfigAuditLogRepository interface {
	// Create operations
	Create(ctx context.Context, log *models.CreditConfigAuditLog) error
	BatchCreate(ctx context.Context, logs []*models.CreditConfigAuditLog) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.CreditConfigAuditLog, error)
	GetByTable(ctx context.Context, configTable string, limit, offset int) ([]*models.CreditConfigAuditLog, error)
	GetByConfigID(ctx context.Context, configID string, limit, offset int) ([]*models.CreditConfigAuditLog, error)
	GetByAction(ctx context.Context, action string, limit, offset int) ([]*models.CreditConfigAuditLog, error)
	GetByDateRange(ctx context.Context, startDate, endDate time.Time, limit, offset int) ([]*models.CreditConfigAuditLog, error)
	GetAll(ctx context.Context, limit, offset int) ([]*models.CreditConfigAuditLog, error)
}

// creditConfigAuditLogRepository represents a repository for managing credit config audit logs
type creditConfigAuditLogRepository struct {
	db *gorm.DB
}

// NewCreditConfigAuditLogRepository creates a new instance of CreditConfigAuditLogRepository
func NewCreditConfigAuditLogRepository(db *gorm.DB) (CreditConfigAuditLogRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &creditConfigAuditLogRepository{db: db}, nil
}

// --- Create Operations ---

// Create creates a new credit config audit log
func (r *creditConfigAuditLogRepository) Create(ctx context.Context, auditLog *models.CreditConfigAuditLog) error {
	result := r.db.WithContext(ctx).Create(auditLog)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateCreditConfigAuditLog
	}
	return nil
}

// BatchCreate creates multiple credit config audit logs in a single batch
func (r *creditConfigAuditLogRepository) BatchCreate(ctx context.Context, logs []*models.CreditConfigAuditLog) error {
	if len(logs) == 0 {
		return nil
	}

	result := r.db.WithContext(ctx).Create(logs)
	if result.Error != nil {
		log.Printf("CreateBatch: database error: %v", result.Error)
		return ErrFailedToCreateBatchCreditConfigAuditLogs
	}
	return nil
}

// --- Read Operations ---

// GetByID retrieves a credit config audit log by its ID
func (r *creditConfigAuditLogRepository) GetByID(ctx context.Context, id string) (*models.CreditConfigAuditLog, error) {
	var auditLog models.CreditConfigAuditLog
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&auditLog)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrCreditConfigAuditLogNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditConfigAuditLog
	}
	return &auditLog, nil
}

// GetByTable retrieves audit logs by config table
func (r *creditConfigAuditLogRepository) GetByTable(ctx context.Context, configTable string, limit, offset int) ([]*models.CreditConfigAuditLog, error) {
	var logs []*models.CreditConfigAuditLog
	result := r.db.WithContext(ctx).
		Where("config_table = ?", configTable).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs)
	if result.Error != nil {
		log.Printf("GetByTable: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditConfigAuditLogsByTable
	}
	return logs, nil
}

// GetByConfigID retrieves audit logs by config ID
func (r *creditConfigAuditLogRepository) GetByConfigID(ctx context.Context, configID string, limit, offset int) ([]*models.CreditConfigAuditLog, error) {
	var logs []*models.CreditConfigAuditLog
	result := r.db.WithContext(ctx).
		Where("config_id = ?", configID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs)
	if result.Error != nil {
		log.Printf("GetByConfigID: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditConfigAuditLogsByConfigID
	}
	return logs, nil
}

// GetByAction retrieves audit logs by action type
func (r *creditConfigAuditLogRepository) GetByAction(ctx context.Context, action string, limit, offset int) ([]*models.CreditConfigAuditLog, error) {
	var logs []*models.CreditConfigAuditLog
	result := r.db.WithContext(ctx).
		Where("action = ?", action).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs)
	if result.Error != nil {
		log.Printf("GetByAction: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditConfigAuditLogsByAction
	}
	return logs, nil
}

// GetByDateRange retrieves audit logs within a date range
func (r *creditConfigAuditLogRepository) GetByDateRange(ctx context.Context, startDate, endDate time.Time, limit, offset int) ([]*models.CreditConfigAuditLog, error) {
	var logs []*models.CreditConfigAuditLog
	result := r.db.WithContext(ctx).
		Where("created_at BETWEEN ? AND ?", startDate, endDate).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs)
	if result.Error != nil {
		log.Printf("GetByDateRange: database error: %v", result.Error)
		return nil, ErrFailedToGetCreditConfigAuditLogsByDateRange
	}
	return logs, nil
}

// GetAll retrieves all credit config audit logs
func (r *creditConfigAuditLogRepository) GetAll(ctx context.Context, limit, offset int) ([]*models.CreditConfigAuditLog, error) {
	var logs []*models.CreditConfigAuditLog
	result := r.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs)
	if result.Error != nil {
		log.Printf("GetAll: database error: %v", result.Error)
		return nil, ErrFailedToGetAllCreditConfigAuditLogs
	}
	return logs, nil
}
