package creditscore

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
)

const (
	MinScore = 0
	MaxScore = 1000
)

// Service defines the interface for credit score business logic operations
type Service interface {
	// Credit score management
	Create(ctx context.Context, req CreateCreditScoreRequest) (*CreditScoreResponse, error)
	GetLatestByUserID(ctx context.Context, userID string) (*CreditScoreResponse, error)
	GetHistoryByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[CreditScoreResponse], error)
	GetUsersNeedingRecalculation(ctx context.Context, pagination services.Pagination) ([]string, error)
	Update(ctx context.Context, id string, req UpdateCreditScoreRequest) (*CreditScoreResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error
}

// service implements the Service interface
type service struct {
	repo repository.CreditScoreRepository
}

// NewService creates a new credit score service instance
func NewService(repo repository.CreditScoreRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new credit score with business validation
func (s *service) Create(ctx context.Context, req CreateCreditScoreRequest) (*CreditScoreResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Set default max concurrent loans if not provided
	maxConcurrentLoans := req.MaxConcurrentLoans
	if maxConcurrentLoans <= 0 {
		maxConcurrentLoans = 1
	}

	// Create credit score model
	creditScore := &models.CreditScore{
		UserID:               req.UserID,
		Score:                req.Score,
		ScoreVersion:         req.ScoreVersion,
		TotalLoans:           req.TotalLoans,
		SuccessfulRepayments: req.SuccessfulRepayments,
		Defaults:             req.Defaults,
		CurrentOutstanding:   req.CurrentOutstanding,
		DaysOverdue:          req.DaysOverdue,
		MaxLoanAmount:        req.MaxLoanAmount,
		MaxConcurrentLoans:   maxConcurrentLoans,
		CalculatedAt:         time.Now(),
		ExpiresAt:            req.ExpiresAt,
	}

	// Create credit score in database
	if err := s.repo.Create(ctx, creditScore); err != nil {
		log.Printf("Create: failed to create credit score: %v", err)
		return nil, err
	}

	return toCreditScoreResponse(creditScore), nil
}

// GetLatestByUserID retrieves the latest credit score for a user
func (s *service) GetLatestByUserID(ctx context.Context, userID string) (*CreditScoreResponse, error) {
	creditScore, err := s.repo.GetLatestByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrCreditScoreNotFound) {
			return nil, ErrCreditScoreNotFound
		}
		log.Printf("GetLatestByUserID: failed to get credit score: %v", err)
		return nil, err
	}

	return toCreditScoreResponse(creditScore), nil
}

// GetHistoryByUserID retrieves credit score history for a user with pagination
func (s *service) GetHistoryByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[CreditScoreResponse], error) {
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

	scores, err := s.repo.GetHistoryByUserID(ctx, userID, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetHistoryByUserID: failed to get credit score history: %v", err)
		return nil, err
	}

	responses := make([]CreditScoreResponse, len(scores))
	for i, score := range scores {
		responses[i] = *toCreditScoreResponse(score)
	}

	return &services.PaginatedResponse[CreditScoreResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetUsersNeedingRecalculation retrieves user IDs that need score recalculation
func (s *service) GetUsersNeedingRecalculation(ctx context.Context, pagination services.Pagination) ([]string, error) {
	if pagination.Page <= 0 {
		pagination.Page = 1
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 100
	}
	if pagination.PageSize > 1000 {
		pagination.PageSize = 1000
	}

	offset := (pagination.Page - 1) * pagination.PageSize

	userIDs, err := s.repo.GetUsersNeedingRecalculation(ctx, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetUsersNeedingRecalculation: failed to get users: %v", err)
		return nil, err
	}

	return userIDs, nil
}

// Update updates credit score information
func (s *service) Update(ctx context.Context, id string, req UpdateCreditScoreRequest) (*CreditScoreResponse, error) {
	creditScore, err := s.repo.GetLatestByUserID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCreditScoreNotFound) {
			return nil, ErrCreditScoreNotFound
		}
		log.Printf("Update: failed to get credit score: %v", err)
		return nil, err
	}

	// Validate score if being updated
	if req.Score != nil {
		if *req.Score < MinScore || *req.Score > MaxScore {
			return nil, ErrScoreOutOfRange
		}
		creditScore.Score = *req.Score
	}

	// Update fields
	if req.ScoreVersion != nil {
		creditScore.ScoreVersion = *req.ScoreVersion
	}
	if req.TotalLoans != nil {
		creditScore.TotalLoans = *req.TotalLoans
	}
	if req.SuccessfulRepayments != nil {
		creditScore.SuccessfulRepayments = *req.SuccessfulRepayments
	}
	if req.Defaults != nil {
		creditScore.Defaults = *req.Defaults
	}
	if req.CurrentOutstanding != nil {
		creditScore.CurrentOutstanding = *req.CurrentOutstanding
	}
	if req.DaysOverdue != nil {
		creditScore.DaysOverdue = *req.DaysOverdue
	}
	if req.MaxLoanAmount != nil {
		creditScore.MaxLoanAmount = req.MaxLoanAmount
	}
	if req.MaxConcurrentLoans != nil {
		creditScore.MaxConcurrentLoans = *req.MaxConcurrentLoans
	}

	creditScore.CalculatedAt = time.Now()

	// Update in database
	if err := s.repo.Update(ctx, creditScore); err != nil {
		log.Printf("Update: failed to update credit score: %v", err)
		return nil, err
	}

	return toCreditScoreResponse(creditScore), nil
}

// Delete soft deletes a credit score
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrCreditScoreNotFound) {
			return ErrCreditScoreNotFound
		}
		log.Printf("Delete: failed to delete credit score: %v", err)
		return err
	}

	return nil
}

// Restore restores a soft-deleted credit score
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrCreditScoreNotFound) {
			return ErrCreditScoreNotFound
		}
		log.Printf("Restore: failed to restore credit score: %v", err)
		return err
	}

	return nil
}

// DeleteByUserID deletes all credit scores for a user
func (s *service) DeleteByUserID(ctx context.Context, userID string) error {
	if err := s.repo.DeleteByUserID(ctx, userID); err != nil {
		log.Printf("DeleteByUserID: failed to delete credit scores: %v", err)
		return err
	}

	return nil
}

// --- Helper functions ---

// validateCreateRequest validates the create credit score request
func (s *service) validateCreateRequest(req CreateCreditScoreRequest) error {
	if req.UserID == "" {
		return ErrInvalidInput
	}

	if req.Score < MinScore || req.Score > MaxScore {
		return ErrScoreOutOfRange
	}

	if req.ScoreVersion == "" {
		return ErrInvalidScoreVersion
	}

	return nil
}

// toCreditScoreResponse converts a credit score model to response DTO
func toCreditScoreResponse(score *models.CreditScore) *CreditScoreResponse {
	return &CreditScoreResponse{
		ID:                   score.ID,
		UserID:               score.UserID,
		Score:                score.Score,
		ScoreVersion:         score.ScoreVersion,
		TotalLoans:           score.TotalLoans,
		SuccessfulRepayments: score.SuccessfulRepayments,
		Defaults:             score.Defaults,
		CurrentOutstanding:   score.CurrentOutstanding,
		DaysOverdue:          score.DaysOverdue,
		MaxLoanAmount:        score.MaxLoanAmount,
		MaxConcurrentLoans:   score.MaxConcurrentLoans,
		CalculatedAt:         score.CalculatedAt,
		ExpiresAt:            score.ExpiresAt,
		IsExpired:            score.IsExpired(),
		CreatedAt:            score.CreatedAt,
		UpdatedAt:            score.UpdatedAt,
	}
}
