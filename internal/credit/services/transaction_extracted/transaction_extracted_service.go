package transactionextracted

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
)

// Service defines the interface for transaction extracted business logic operations
type Service interface {
	// Transaction management
	Create(ctx context.Context, req CreateTransactionExtractedRequest) (*TransactionExtractedResponse, error)
	BatchCreate(ctx context.Context, reqs []CreateTransactionExtractedRequest) ([]*TransactionExtractedResponse, error)
	GetByID(ctx context.Context, id string) (*TransactionExtractedResponse, error)
	GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[TransactionExtractedResponse], error)
	GetByDocumentID(ctx context.Context, documentID string) ([]*TransactionExtractedResponse, error)
	GetByDateRange(ctx context.Context, userID string, startDate, endDate time.Time, pagination services.Pagination) (*services.PaginatedResponse[TransactionExtractedResponse], error)
	GetByCategory(ctx context.Context, userID, category string, pagination services.Pagination) (*services.PaginatedResponse[TransactionExtractedResponse], error)
	GetBySource(ctx context.Context, userID, source string, pagination services.Pagination) (*services.PaginatedResponse[TransactionExtractedResponse], error)
	GetIncomeTransactions(ctx context.Context, userID string, startDate, endDate time.Time) ([]*TransactionExtractedResponse, error)
	GetExpenseTransactions(ctx context.Context, userID string, startDate, endDate time.Time) ([]*TransactionExtractedResponse, error)
	GetRecentTransactions(ctx context.Context, userID string, limit int) ([]*TransactionExtractedResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error
	DeleteByDocumentID(ctx context.Context, documentID string) error

	// Aggregations
	CountByUserID(ctx context.Context, userID string) (int64, error)
	CountBySource(ctx context.Context, userID, source string) (int64, error)
	CountByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) (int64, error)
	GetSummary(ctx context.Context, userID string, startDate, endDate time.Time) (*TransactionSummaryResponse, error)
}

// service implements the Service interface
type service struct {
	repo repository.TransactionExtractedRepository
}

// NewService creates a new transaction extracted service instance
func NewService(repo repository.TransactionExtractedRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new extracted transaction with business validation
func (s *service) Create(ctx context.Context, req CreateTransactionExtractedRequest) (*TransactionExtractedResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Create transaction model
	tx := &models.TransactionExtracted{
		UserID:          req.UserID,
		DocumentID:      req.DocumentID,
		TransactionDate: req.TransactionDate,
		TransactionType: req.TransactionType,
		Amount:          req.Amount,
		Counterparty:    req.Counterparty,
		Category:        req.Category,
		Source:          req.Source,
		Description:     req.Description,
		BalanceAfter:    req.BalanceAfter,
	}

	// Create transaction in database
	if err := s.repo.Create(ctx, tx); err != nil {
		log.Printf("Create: failed to create transaction: %v", err)
		return nil, err
	}

	return toTransactionExtractedResponse(tx), nil
}

// BatchCreate creates multiple extracted transactions
func (s *service) BatchCreate(ctx context.Context, reqs []CreateTransactionExtractedRequest) ([]*TransactionExtractedResponse, error) {
	if len(reqs) == 0 {
		return []*TransactionExtractedResponse{}, nil
	}

	txs := make([]*models.TransactionExtracted, len(reqs))
	for i, req := range reqs {
		if err := s.validateCreateRequest(req); err != nil {
			return nil, err
		}

		txs[i] = &models.TransactionExtracted{
			UserID:          req.UserID,
			DocumentID:      req.DocumentID,
			TransactionDate: req.TransactionDate,
			TransactionType: req.TransactionType,
			Amount:          req.Amount,
			Counterparty:    req.Counterparty,
			Category:        req.Category,
			Source:          req.Source,
			Description:     req.Description,
			BalanceAfter:    req.BalanceAfter,
		}
	}

	if err := s.repo.BatchCreate(ctx, txs); err != nil {
		log.Printf("BatchCreate: failed to create transactions: %v", err)
		return nil, err
	}

	responses := make([]*TransactionExtractedResponse, len(txs))
	for i, tx := range txs {
		responses[i] = toTransactionExtractedResponse(tx)
	}

	return responses, nil
}

// GetByID retrieves an extracted transaction by ID
func (s *service) GetByID(ctx context.Context, id string) (*TransactionExtractedResponse, error) {
	tx, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return nil, ErrTransactionNotFound
		}
		log.Printf("GetByID: failed to get transaction: %v", err)
		return nil, err
	}

	return toTransactionExtractedResponse(tx), nil
}

// GetByUserID retrieves extracted transactions by user ID with pagination
func (s *service) GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[TransactionExtractedResponse], error) {
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

	txs, err := s.repo.GetByUserID(ctx, userID, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByUserID: failed to get transactions: %v", err)
		return nil, err
	}

	responses := make([]TransactionExtractedResponse, len(txs))
	for i, tx := range txs {
		responses[i] = *toTransactionExtractedResponse(tx)
	}

	return &services.PaginatedResponse[TransactionExtractedResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetByDocumentID retrieves all transactions extracted from a specific document
func (s *service) GetByDocumentID(ctx context.Context, documentID string) ([]*TransactionExtractedResponse, error) {
	txs, err := s.repo.GetByDocumentID(ctx, documentID)
	if err != nil {
		log.Printf("GetByDocumentID: failed to get transactions: %v", err)
		return nil, err
	}

	responses := make([]*TransactionExtractedResponse, len(txs))
	for i, tx := range txs {
		responses[i] = toTransactionExtractedResponse(tx)
	}

	return responses, nil
}

// GetByDateRange retrieves transactions within a date range with pagination
func (s *service) GetByDateRange(ctx context.Context, userID string, startDate, endDate time.Time, pagination services.Pagination) (*services.PaginatedResponse[TransactionExtractedResponse], error) {
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

	txs, err := s.repo.GetByDateRange(ctx, userID, startDate, endDate, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByDateRange: failed to get transactions: %v", err)
		return nil, err
	}

	responses := make([]TransactionExtractedResponse, len(txs))
	for i, tx := range txs {
		responses[i] = *toTransactionExtractedResponse(tx)
	}

	return &services.PaginatedResponse[TransactionExtractedResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetByCategory retrieves transactions by category with pagination
func (s *service) GetByCategory(ctx context.Context, userID, category string, pagination services.Pagination) (*services.PaginatedResponse[TransactionExtractedResponse], error) {
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

	txs, err := s.repo.GetByCategory(ctx, userID, category, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByCategory: failed to get transactions: %v", err)
		return nil, err
	}

	responses := make([]TransactionExtractedResponse, len(txs))
	for i, tx := range txs {
		responses[i] = *toTransactionExtractedResponse(tx)
	}

	return &services.PaginatedResponse[TransactionExtractedResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetBySource retrieves transactions by source with pagination
func (s *service) GetBySource(ctx context.Context, userID, source string, pagination services.Pagination) (*services.PaginatedResponse[TransactionExtractedResponse], error) {
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

	txs, err := s.repo.GetBySource(ctx, userID, source, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetBySource: failed to get transactions: %v", err)
		return nil, err
	}

	responses := make([]TransactionExtractedResponse, len(txs))
	for i, tx := range txs {
		responses[i] = *toTransactionExtractedResponse(tx)
	}

	return &services.PaginatedResponse[TransactionExtractedResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetIncomeTransactions retrieves income transactions within a date range
func (s *service) GetIncomeTransactions(ctx context.Context, userID string, startDate, endDate time.Time) ([]*TransactionExtractedResponse, error) {
	if endDate.Before(startDate) {
		return nil, ErrInvalidDateRange
	}

	txs, err := s.repo.GetIncomeTransactions(ctx, userID, startDate, endDate)
	if err != nil {
		log.Printf("GetIncomeTransactions: failed to get transactions: %v", err)
		return nil, err
	}

	responses := make([]*TransactionExtractedResponse, len(txs))
	for i, tx := range txs {
		responses[i] = toTransactionExtractedResponse(tx)
	}

	return responses, nil
}

// GetExpenseTransactions retrieves expense transactions within a date range
func (s *service) GetExpenseTransactions(ctx context.Context, userID string, startDate, endDate time.Time) ([]*TransactionExtractedResponse, error) {
	if endDate.Before(startDate) {
		return nil, ErrInvalidDateRange
	}

	txs, err := s.repo.GetExpenseTransactions(ctx, userID, startDate, endDate)
	if err != nil {
		log.Printf("GetExpenseTransactions: failed to get transactions: %v", err)
		return nil, err
	}

	responses := make([]*TransactionExtractedResponse, len(txs))
	for i, tx := range txs {
		responses[i] = toTransactionExtractedResponse(tx)
	}

	return responses, nil
}

// GetRecentTransactions retrieves the most recent transactions for a user
func (s *service) GetRecentTransactions(ctx context.Context, userID string, limit int) ([]*TransactionExtractedResponse, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	txs, err := s.repo.GetRecentTransactions(ctx, userID, limit)
	if err != nil {
		log.Printf("GetRecentTransactions: failed to get transactions: %v", err)
		return nil, err
	}

	responses := make([]*TransactionExtractedResponse, len(txs))
	for i, tx := range txs {
		responses[i] = toTransactionExtractedResponse(tx)
	}

	return responses, nil
}

// Delete soft deletes an extracted transaction
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return ErrTransactionNotFound
		}
		log.Printf("Delete: failed to delete transaction: %v", err)
		return err
	}

	return nil
}

// Restore restores a soft-deleted extracted transaction
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrTransactionNotFound) {
			return ErrTransactionNotFound
		}
		log.Printf("Restore: failed to restore transaction: %v", err)
		return err
	}

	return nil
}

// DeleteByDocumentID deletes all transactions associated with a document
func (s *service) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if err := s.repo.DeleteByDocumentID(ctx, documentID); err != nil {
		log.Printf("DeleteByDocumentID: failed to delete transactions: %v", err)
		return err
	}

	return nil
}

// CountByUserID counts total transactions for a user
func (s *service) CountByUserID(ctx context.Context, userID string) (int64, error) {
	count, err := s.repo.CountByUserID(ctx, userID)
	if err != nil {
		log.Printf("CountByUserID: failed to count transactions: %v", err)
		return 0, err
	}
	return count, nil
}

// CountBySource counts transactions by source for a user
func (s *service) CountBySource(ctx context.Context, userID, source string) (int64, error) {
	count, err := s.repo.CountBySource(ctx, userID, source)
	if err != nil {
		log.Printf("CountBySource: failed to count transactions: %v", err)
		return 0, err
	}
	return count, nil
}

// CountByDateRange counts transactions within a date range for a user
func (s *service) CountByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) (int64, error) {
	if endDate.Before(startDate) {
		return 0, ErrInvalidDateRange
	}

	count, err := s.repo.CountByDateRange(ctx, userID, startDate, endDate)
	if err != nil {
		log.Printf("CountByDateRange: failed to count transactions: %v", err)
		return 0, err
	}
	return count, nil
}

// GetSummary retrieves summary information for a user within a date range
func (s *service) GetSummary(ctx context.Context, userID string, startDate, endDate time.Time) (*TransactionSummaryResponse, error) {
	if endDate.Before(startDate) {
		return nil, ErrInvalidDateRange
	}

	summary, err := s.repo.GetSummary(ctx, userID, startDate, endDate)
	if err != nil {
		log.Printf("GetSummary: failed to get summary: %v", err)
		return nil, err
	}

	return &TransactionSummaryResponse{
		TotalCount:          summary["total_count"].(int),
		TotalIncome:         summary["total_income"].(int64),
		TotalExpenses:       summary["total_expenses"].(int64),
		NetCashflow:         summary["net_cashflow"].(int64),
		MonthsWithActivity:  summary["months_with_activity"].(int),
		UniqueIncomeSources: summary["unique_income_sources"].(int),
		AvgMonthlyIncome:    summary["avg_monthly_income"].(int64),
		AvgMonthlyExpenses:  summary["avg_monthly_expenses"].(int64),
	}, nil
}

// --- Helper functions ---

// validateCreateRequest validates the create extracted transaction request
func (s *service) validateCreateRequest(req CreateTransactionExtractedRequest) error {
	if req.UserID == "" {
		return ErrInvalidInput
	}

	if req.DocumentID == "" {
		return ErrInvalidInput
	}

	if req.TransactionDate.IsZero() {
		return ErrInvalidDateRange
	}

	if req.TransactionType == "" {
		return ErrInvalidInput
	}

	if req.Amount <= 0 {
		return ErrInvalidAmount
	}

	if req.Source == "" {
		return ErrInvalidSource
	}

	return nil
}

// toTransactionExtractedResponse converts a transaction extracted model to response DTO
func toTransactionExtractedResponse(tx *models.TransactionExtracted) *TransactionExtractedResponse {
	return &TransactionExtractedResponse{
		ID:              tx.ID,
		UserID:          tx.UserID,
		DocumentID:      tx.DocumentID,
		TransactionDate: tx.TransactionDate,
		TransactionType: tx.TransactionType,
		Amount:          tx.Amount,
		Counterparty:    tx.Counterparty,
		Category:        tx.Category,
		Source:          tx.Source,
		Description:     tx.Description,
		BalanceAfter:    tx.BalanceAfter,
		CreatedAt:       tx.CreatedAt,
		UpdatedAt:       tx.UpdatedAt,
	}
}
