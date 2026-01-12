package creditconfigauditlog

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
)

// Valid audit actions
var validActions = map[string]bool{
	models.AuditActionCreate:     true,
	models.AuditActionUpdate:     true,
	models.AuditActionDelete:     true,
	models.AuditActionActivate:   true,
	models.AuditActionDeactivate: true,
}

// Service defines the interface for credit config audit log business logic operations
type Service interface {
	// Audit log management
	Create(ctx context.Context, req CreateCreditConfigAuditLogRequest) (*CreditConfigAuditLogResponse, error)
	BatchCreate(ctx context.Context, reqs []CreateCreditConfigAuditLogRequest) ([]*CreditConfigAuditLogResponse, error)
	GetByID(ctx context.Context, id string) (*CreditConfigAuditLogResponse, error)
	GetByTable(ctx context.Context, configTable string, pagination services.Pagination) (*services.PaginatedResponse[CreditConfigAuditLogResponse], error)
	GetByConfigID(ctx context.Context, configID string, pagination services.Pagination) (*services.PaginatedResponse[CreditConfigAuditLogResponse], error)
	GetByAction(ctx context.Context, action string, pagination services.Pagination) (*services.PaginatedResponse[CreditConfigAuditLogResponse], error)
	GetByDateRange(ctx context.Context, startDate, endDate time.Time, pagination services.Pagination) (*services.PaginatedResponse[CreditConfigAuditLogResponse], error)
	GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[CreditConfigAuditLogResponse], error)
}

// service implements the Service interface
type service struct {
	repo repository.CreditConfigAuditLogRepository
}

// NewService creates a new credit config audit log service instance
func NewService(repo repository.CreditConfigAuditLogRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new audit log entry with business validation
func (s *service) Create(ctx context.Context, req CreateCreditConfigAuditLogRequest) (*CreditConfigAuditLogResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Create audit log model
	auditLog := &models.CreditConfigAuditLog{
		ConfigTable:  req.ConfigTable,
		ConfigID:     req.ConfigID,
		Action:       req.Action,
		OldValues:    req.OldValues,
		NewValues:    req.NewValues,
		ChangedBy:    req.ChangedBy,
		ChangeReason: req.ChangeReason,
	}

	// Create audit log in database
	if err := s.repo.Create(ctx, auditLog); err != nil {
		log.Printf("Create: failed to create audit log: %v", err)
		return nil, err
	}

	return toCreditConfigAuditLogResponse(auditLog), nil
}

// BatchCreate creates multiple audit log entries
func (s *service) BatchCreate(ctx context.Context, reqs []CreateCreditConfigAuditLogRequest) ([]*CreditConfigAuditLogResponse, error) {
	if len(reqs) == 0 {
		return []*CreditConfigAuditLogResponse{}, nil
	}

	logs := make([]*models.CreditConfigAuditLog, len(reqs))
	for i, req := range reqs {
		if err := s.validateCreateRequest(req); err != nil {
			return nil, err
		}

		logs[i] = &models.CreditConfigAuditLog{
			ConfigTable:  req.ConfigTable,
			ConfigID:     req.ConfigID,
			Action:       req.Action,
			OldValues:    req.OldValues,
			NewValues:    req.NewValues,
			ChangedBy:    req.ChangedBy,
			ChangeReason: req.ChangeReason,
		}
	}

	if err := s.repo.BatchCreate(ctx, logs); err != nil {
		log.Printf("BatchCreate: failed to create audit logs: %v", err)
		return nil, err
	}

	responses := make([]*CreditConfigAuditLogResponse, len(logs))
	for i, auditLog := range logs {
		responses[i] = toCreditConfigAuditLogResponse(auditLog)
	}

	return responses, nil
}

// GetByID retrieves an audit log by ID
func (s *service) GetByID(ctx context.Context, id string) (*CreditConfigAuditLogResponse, error) {
	auditLog, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCreditConfigAuditLogNotFound) {
			return nil, ErrCreditConfigAuditLogNotFound
		}
		log.Printf("GetByID: failed to get audit log: %v", err)
		return nil, err
	}

	return toCreditConfigAuditLogResponse(auditLog), nil
}

// GetByTable retrieves audit logs by config table with pagination
func (s *service) GetByTable(ctx context.Context, configTable string, pagination services.Pagination) (*services.PaginatedResponse[CreditConfigAuditLogResponse], error) {
	if pagination.Page <= 0 {
		pagination.Page = 1
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	if pagination.PageSize > 100 {
		pagination.PageSize = 100
	}

	offset := (pagination.Page - 1) * pagination.PageSize

	logs, err := s.repo.GetByTable(ctx, configTable, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByTable: failed to get audit logs: %v", err)
		return nil, err
	}

	responses := make([]CreditConfigAuditLogResponse, len(logs))
	for i, auditLog := range logs {
		responses[i] = *toCreditConfigAuditLogResponse(auditLog)
	}

	return &services.PaginatedResponse[CreditConfigAuditLogResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetByConfigID retrieves audit logs by config ID with pagination
func (s *service) GetByConfigID(ctx context.Context, configID string, pagination services.Pagination) (*services.PaginatedResponse[CreditConfigAuditLogResponse], error) {
	if pagination.Page <= 0 {
		pagination.Page = 1
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	if pagination.PageSize > 100 {
		pagination.PageSize = 100
	}

	offset := (pagination.Page - 1) * pagination.PageSize

	logs, err := s.repo.GetByConfigID(ctx, configID, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByConfigID: failed to get audit logs: %v", err)
		return nil, err
	}

	responses := make([]CreditConfigAuditLogResponse, len(logs))
	for i, auditLog := range logs {
		responses[i] = *toCreditConfigAuditLogResponse(auditLog)
	}

	return &services.PaginatedResponse[CreditConfigAuditLogResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetByAction retrieves audit logs by action with pagination
func (s *service) GetByAction(ctx context.Context, action string, pagination services.Pagination) (*services.PaginatedResponse[CreditConfigAuditLogResponse], error) {
	if !validActions[action] {
		return nil, ErrInvalidAction
	}

	if pagination.Page <= 0 {
		pagination.Page = 1
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	if pagination.PageSize > 100 {
		pagination.PageSize = 100
	}

	offset := (pagination.Page - 1) * pagination.PageSize

	logs, err := s.repo.GetByAction(ctx, action, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByAction: failed to get audit logs: %v", err)
		return nil, err
	}

	responses := make([]CreditConfigAuditLogResponse, len(logs))
	for i, auditLog := range logs {
		responses[i] = *toCreditConfigAuditLogResponse(auditLog)
	}

	return &services.PaginatedResponse[CreditConfigAuditLogResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetByDateRange retrieves audit logs within a date range with pagination
func (s *service) GetByDateRange(ctx context.Context, startDate, endDate time.Time, pagination services.Pagination) (*services.PaginatedResponse[CreditConfigAuditLogResponse], error) {
	if endDate.Before(startDate) {
		return nil, ErrInvalidDateRange
	}

	if pagination.Page <= 0 {
		pagination.Page = 1
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	if pagination.PageSize > 100 {
		pagination.PageSize = 100
	}

	offset := (pagination.Page - 1) * pagination.PageSize

	logs, err := s.repo.GetByDateRange(ctx, startDate, endDate, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByDateRange: failed to get audit logs: %v", err)
		return nil, err
	}

	responses := make([]CreditConfigAuditLogResponse, len(logs))
	for i, auditLog := range logs {
		responses[i] = *toCreditConfigAuditLogResponse(auditLog)
	}

	return &services.PaginatedResponse[CreditConfigAuditLogResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetAll retrieves all audit logs with pagination
func (s *service) GetAll(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[CreditConfigAuditLogResponse], error) {
	if pagination.Page <= 0 {
		pagination.Page = 1
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	if pagination.PageSize > 100 {
		pagination.PageSize = 100
	}

	offset := (pagination.Page - 1) * pagination.PageSize

	logs, err := s.repo.GetAll(ctx, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetAll: failed to get audit logs: %v", err)
		return nil, err
	}

	responses := make([]CreditConfigAuditLogResponse, len(logs))
	for i, auditLog := range logs {
		responses[i] = *toCreditConfigAuditLogResponse(auditLog)
	}

	return &services.PaginatedResponse[CreditConfigAuditLogResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// --- Helper functions ---

// validateCreateRequest validates the create audit log request
func (s *service) validateCreateRequest(req CreateCreditConfigAuditLogRequest) error {
	if req.ConfigTable == "" {
		return ErrInvalidConfigTable
	}

	if req.ConfigID == "" {
		return ErrInvalidConfigID
	}

	if !validActions[req.Action] {
		return ErrInvalidAction
	}

	return nil
}

// toCreditConfigAuditLogResponse converts an audit log model to response DTO
func toCreditConfigAuditLogResponse(auditLog *models.CreditConfigAuditLog) *CreditConfigAuditLogResponse {
	return &CreditConfigAuditLogResponse{
		ID:           auditLog.ID,
		ConfigTable:  auditLog.ConfigTable,
		ConfigID:     auditLog.ConfigID,
		Action:       auditLog.Action,
		OldValues:    auditLog.OldValues,
		NewValues:    auditLog.NewValues,
		ChangedBy:    auditLog.ChangedBy,
		ChangeReason: auditLog.ChangeReason,
		CreatedAt:    auditLog.CreatedAt,
	}
}
