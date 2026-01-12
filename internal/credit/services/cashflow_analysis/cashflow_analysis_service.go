package cashflowanalysis

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
	"gorm.io/datatypes"
)

// Service defines the interface for cashflow analysis business logic operations
type Service interface {
	// Cashflow analysis management
	Create(ctx context.Context, req CreateCashflowAnalysisRequest) (*CashflowAnalysisResponse, error)
	BatchCreate(ctx context.Context, reqs []CreateCashflowAnalysisRequest) ([]*CashflowAnalysisResponse, error)
	GetByID(ctx context.Context, id string) (*CashflowAnalysisResponse, error)
	GetLatestByUserID(ctx context.Context, userID string) (*CashflowAnalysisResponse, error)
	GetAllByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[CashflowAnalysisResponse], error)
	GetByUserIDAndPeriod(ctx context.Context, userID string, start, end time.Time) (*CashflowAnalysisResponse, error)
	GetNonStaleByUserID(ctx context.Context, userID string, maxAge time.Duration) (*CashflowAnalysisResponse, error)
	GetPositiveCashflowByUserID(ctx context.Context, userID string) ([]*CashflowAnalysisResponse, error)
	Update(ctx context.Context, id string, req UpdateCashflowAnalysisRequest) (*CashflowAnalysisResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error

	// Aggregations
	GetAverageIncomeByUserID(ctx context.Context, userID string) (int64, error)
	GetAverageExpensesByUserID(ctx context.Context, userID string) (int64, error)
	GetTotalTransactionCountByUserID(ctx context.Context, userID string) (int64, error)
	CountByUserID(ctx context.Context, userID string) (int64, error)

	// Comparison
	CompareTwoMostRecent(ctx context.Context, userID string) (*CashflowComparisonResponse, error)
}

// service implements the Service interface
type service struct {
	repo repository.CashflowAnalysisRepository
}

// NewService creates a new cashflow analysis service instance
func NewService(repo repository.CashflowAnalysisRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new cashflow analysis with business validation
func (s *service) Create(ctx context.Context, req CreateCashflowAnalysisRequest) (*CashflowAnalysisResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Create cashflow analysis model
	analysis := &models.CashflowAnalysis{
		UserID:                 req.UserID,
		AnalysisPeriodStart:    req.AnalysisPeriodStart,
		AnalysisPeriodEnd:      req.AnalysisPeriodEnd,
		TotalIncome:            req.TotalIncome,
		TotalExpenses:          req.TotalExpenses,
		IncomeStabilityScore:   req.IncomeStabilityScore,
		ExpenseRegularityScore: req.ExpenseRegularityScore,
		TransactionCount:       req.TransactionCount,
		UniqueIncomeSources:    req.UniqueIncomeSources,
		IncomeSources:          toJSONField(req.IncomeSources),
		ExpenseCategories:      toJSONField(req.ExpenseCategories),
		SeasonalPattern:        toJSONField(req.SeasonalPattern),
		RiskFlags:              toJSONField(req.RiskFlags),
	}

	// Create analysis in database (repository handles derived calculations)
	if err := s.repo.Create(ctx, analysis); err != nil {
		log.Printf("Create: failed to create cashflow analysis: %v", err)
		return nil, err
	}

	return toCashflowAnalysisResponse(analysis), nil
}

// BatchCreate creates multiple cashflow analyses
func (s *service) BatchCreate(ctx context.Context, reqs []CreateCashflowAnalysisRequest) ([]*CashflowAnalysisResponse, error) {
	if len(reqs) == 0 {
		return []*CashflowAnalysisResponse{}, nil
	}

	analyses := make([]*models.CashflowAnalysis, len(reqs))
	for i, req := range reqs {
		if err := s.validateCreateRequest(req); err != nil {
			return nil, err
		}

		analyses[i] = &models.CashflowAnalysis{
			UserID:                 req.UserID,
			AnalysisPeriodStart:    req.AnalysisPeriodStart,
			AnalysisPeriodEnd:      req.AnalysisPeriodEnd,
			TotalIncome:            req.TotalIncome,
			TotalExpenses:          req.TotalExpenses,
			IncomeStabilityScore:   req.IncomeStabilityScore,
			ExpenseRegularityScore: req.ExpenseRegularityScore,
			TransactionCount:       req.TransactionCount,
			UniqueIncomeSources:    req.UniqueIncomeSources,
			IncomeSources:          toJSONField(req.IncomeSources),
			ExpenseCategories:      toJSONField(req.ExpenseCategories),
			SeasonalPattern:        toJSONField(req.SeasonalPattern),
			RiskFlags:              toJSONField(req.RiskFlags),
		}
	}

	if err := s.repo.BatchCreate(ctx, analyses); err != nil {
		log.Printf("BatchCreate: failed to create analyses: %v", err)
		return nil, err
	}

	responses := make([]*CashflowAnalysisResponse, len(analyses))
	for i, analysis := range analyses {
		responses[i] = toCashflowAnalysisResponse(analysis)
	}

	return responses, nil
}

// GetByID retrieves a cashflow analysis by ID
func (s *service) GetByID(ctx context.Context, id string) (*CashflowAnalysisResponse, error) {
	analysis, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCashflowAnalysisNotFound) {
			return nil, ErrCashflowAnalysisNotFound
		}
		log.Printf("GetByID: failed to get cashflow analysis: %v", err)
		return nil, err
	}

	return toCashflowAnalysisResponse(analysis), nil
}

// GetLatestByUserID retrieves the latest cashflow analysis for a user
func (s *service) GetLatestByUserID(ctx context.Context, userID string) (*CashflowAnalysisResponse, error) {
	analysis, err := s.repo.GetLatestByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrCashflowAnalysisNotFound) {
			return nil, ErrCashflowAnalysisNotFound
		}
		log.Printf("GetLatestByUserID: failed to get cashflow analysis: %v", err)
		return nil, err
	}

	return toCashflowAnalysisResponse(analysis), nil
}

// GetAllByUserID retrieves all cashflow analyses for a user with pagination
func (s *service) GetAllByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[CashflowAnalysisResponse], error) {
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

	analyses, err := s.repo.GetAllByUserID(ctx, userID, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetAllByUserID: failed to get cashflow analyses: %v", err)
		return nil, err
	}

	responses := make([]CashflowAnalysisResponse, len(analyses))
	for i, analysis := range analyses {
		responses[i] = *toCashflowAnalysisResponse(analysis)
	}

	return &services.PaginatedResponse[CashflowAnalysisResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetByUserIDAndPeriod retrieves analysis for a specific time period
func (s *service) GetByUserIDAndPeriod(ctx context.Context, userID string, start, end time.Time) (*CashflowAnalysisResponse, error) {
	analysis, err := s.repo.GetByUserIDAndPeriod(ctx, userID, start, end)
	if err != nil {
		if errors.Is(err, repository.ErrCashflowAnalysisNotFound) {
			return nil, ErrCashflowAnalysisNotFound
		}
		log.Printf("GetByUserIDAndPeriod: failed to get cashflow analysis: %v", err)
		return nil, err
	}

	return toCashflowAnalysisResponse(analysis), nil
}

// GetNonStaleByUserID retrieves the most recent non-stale analysis
func (s *service) GetNonStaleByUserID(ctx context.Context, userID string, maxAge time.Duration) (*CashflowAnalysisResponse, error) {
	analysis, err := s.repo.GetNonStaleByUserID(ctx, userID, maxAge)
	if err != nil {
		if errors.Is(err, repository.ErrCashflowAnalysisNotFound) {
			return nil, ErrCashflowAnalysisNotFound
		}
		log.Printf("GetNonStaleByUserID: failed to get cashflow analysis: %v", err)
		return nil, err
	}

	return toCashflowAnalysisResponse(analysis), nil
}

// GetPositiveCashflowByUserID retrieves all analyses with positive cashflow
func (s *service) GetPositiveCashflowByUserID(ctx context.Context, userID string) ([]*CashflowAnalysisResponse, error) {
	analyses, err := s.repo.GetPositiveCashflowByUserID(ctx, userID)
	if err != nil {
		log.Printf("GetPositiveCashflowByUserID: failed to get analyses: %v", err)
		return nil, err
	}

	responses := make([]*CashflowAnalysisResponse, len(analyses))
	for i, analysis := range analyses {
		responses[i] = toCashflowAnalysisResponse(analysis)
	}

	return responses, nil
}

// Update updates cashflow analysis information
func (s *service) Update(ctx context.Context, id string, req UpdateCashflowAnalysisRequest) (*CashflowAnalysisResponse, error) {
	analysis, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCashflowAnalysisNotFound) {
			return nil, ErrCashflowAnalysisNotFound
		}
		log.Printf("Update: failed to get cashflow analysis: %v", err)
		return nil, err
	}

	// Update fields
	if req.TotalIncome != nil {
		if *req.TotalIncome < 0 {
			return nil, ErrNegativeIncome
		}
		analysis.TotalIncome = *req.TotalIncome
	}
	if req.TotalExpenses != nil {
		if *req.TotalExpenses < 0 {
			return nil, ErrNegativeExpenses
		}
		analysis.TotalExpenses = *req.TotalExpenses
	}
	if req.IncomeStabilityScore != nil {
		analysis.IncomeStabilityScore = *req.IncomeStabilityScore
	}
	if req.ExpenseRegularityScore != nil {
		analysis.ExpenseRegularityScore = *req.ExpenseRegularityScore
	}
	if req.TransactionCount != nil {
		analysis.TransactionCount = *req.TransactionCount
	}
	if req.UniqueIncomeSources != nil {
		analysis.UniqueIncomeSources = *req.UniqueIncomeSources
	}
	if req.IncomeSources != nil {
		analysis.IncomeSources = toJSONField(req.IncomeSources)
	}
	if req.ExpenseCategories != nil {
		analysis.ExpenseCategories = toJSONField(req.ExpenseCategories)
	}
	if req.SeasonalPattern != nil {
		analysis.SeasonalPattern = toJSONField(req.SeasonalPattern)
	}
	if req.RiskFlags != nil {
		analysis.RiskFlags = toJSONField(req.RiskFlags)
	}

	// Update in database (repository handles recalculations)
	if err := s.repo.Update(ctx, analysis); err != nil {
		log.Printf("Update: failed to update cashflow analysis: %v", err)
		return nil, err
	}

	return toCashflowAnalysisResponse(analysis), nil
}

// Delete soft deletes a cashflow analysis
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrCashflowAnalysisNotFound) {
			return ErrCashflowAnalysisNotFound
		}
		log.Printf("Delete: failed to delete cashflow analysis: %v", err)
		return err
	}

	return nil
}

// Restore restores a soft-deleted cashflow analysis
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrCashflowAnalysisNotFound) {
			return ErrCashflowAnalysisNotFound
		}
		log.Printf("Restore: failed to restore cashflow analysis: %v", err)
		return err
	}

	return nil
}

// GetAverageIncomeByUserID returns average income across all analyses
func (s *service) GetAverageIncomeByUserID(ctx context.Context, userID string) (int64, error) {
	avg, err := s.repo.GetAverageIncomeByUserID(ctx, userID)
	if err != nil {
		log.Printf("GetAverageIncomeByUserID: failed to get average income: %v", err)
		return 0, err
	}
	return avg, nil
}

// GetAverageExpensesByUserID returns average expenses across all analyses
func (s *service) GetAverageExpensesByUserID(ctx context.Context, userID string) (int64, error) {
	avg, err := s.repo.GetAverageExpensesByUserID(ctx, userID)
	if err != nil {
		log.Printf("GetAverageExpensesByUserID: failed to get average expenses: %v", err)
		return 0, err
	}
	return avg, nil
}

// GetTotalTransactionCountByUserID returns total transaction count
func (s *service) GetTotalTransactionCountByUserID(ctx context.Context, userID string) (int64, error) {
	total, err := s.repo.GetTotalTransactionCountByUserID(ctx, userID)
	if err != nil {
		log.Printf("GetTotalTransactionCountByUserID: failed to get count: %v", err)
		return 0, err
	}
	return total, nil
}

// CountByUserID counts the number of analyses for a user
func (s *service) CountByUserID(ctx context.Context, userID string) (int64, error) {
	count, err := s.repo.CountByUserID(ctx, userID)
	if err != nil {
		log.Printf("CountByUserID: failed to count analyses: %v", err)
		return 0, err
	}
	return count, nil
}

// CompareTwoMostRecent compares the two most recent analyses
func (s *service) CompareTwoMostRecent(ctx context.Context, userID string) (*CashflowComparisonResponse, error) {
	analyses, err := s.repo.GetTwoMostRecentByUserID(ctx, userID)
	if err != nil {
		log.Printf("CompareTwoMostRecent: failed to get analyses: %v", err)
		return nil, err
	}

	if len(analyses) == 0 {
		return nil, ErrCashflowAnalysisNotFound
	}

	response := &CashflowComparisonResponse{
		Current: toCashflowAnalysisResponse(analyses[0]),
	}

	if len(analyses) > 1 {
		response.Previous = toCashflowAnalysisResponse(analyses[1])
		response.IncomeChange = analyses[0].TotalIncome - analyses[1].TotalIncome
		response.ExpenseChange = analyses[0].TotalExpenses - analyses[1].TotalExpenses
		response.NetCashflowChange = analyses[0].NetCashflow - analyses[1].NetCashflow
	}

	return response, nil
}

// --- Helper functions ---

// validateCreateRequest validates the create cashflow analysis request
func (s *service) validateCreateRequest(req CreateCashflowAnalysisRequest) error {
	if req.UserID == "" {
		return ErrInvalidInput
	}

	if req.AnalysisPeriodStart.IsZero() || req.AnalysisPeriodEnd.IsZero() {
		return ErrInvalidDateRange
	}

	if req.AnalysisPeriodEnd.Before(req.AnalysisPeriodStart) {
		return ErrInvalidDateRange
	}

	if req.TotalIncome < 0 {
		return ErrNegativeIncome
	}

	if req.TotalExpenses < 0 {
		return ErrNegativeExpenses
	}

	return nil
}

// toCashflowAnalysisResponse converts a cashflow analysis model to response DTO
func toCashflowAnalysisResponse(analysis *models.CashflowAnalysis) *CashflowAnalysisResponse {
	var incomeSources map[string]int64
	var expenseCategories map[string]int64
	var seasonalPattern *string
	var riskFlags []string

	if analysis.IncomeSources != nil {
		err := json.Unmarshal(*analysis.IncomeSources, &incomeSources)
		if err != nil {
			return nil
		}
	}
	if analysis.ExpenseCategories != nil {
		err := json.Unmarshal(*analysis.ExpenseCategories, &expenseCategories)
		if err != nil {
			return nil
		}
	}
	if analysis.SeasonalPattern != nil {
		err := json.Unmarshal(*analysis.SeasonalPattern, &seasonalPattern)
		if err != nil {
			return nil
		}
	}
	if analysis.RiskFlags != nil {
		err := json.Unmarshal(*analysis.RiskFlags, &riskFlags)
		if err != nil {
			return nil
		}
	}

	return &CashflowAnalysisResponse{
		ID:                     analysis.ID,
		UserID:                 analysis.UserID,
		AnalysisPeriodStart:    analysis.AnalysisPeriodStart,
		AnalysisPeriodEnd:      analysis.AnalysisPeriodEnd,
		TotalIncome:            analysis.TotalIncome,
		TotalExpenses:          analysis.TotalExpenses,
		NetCashflow:            analysis.NetCashflow,
		AvgMonthlyIncome:       analysis.AvgMonthlyIncome,
		AvgMonthlyExpenses:     analysis.AvgMonthlyExpenses,
		IncomeStabilityScore:   analysis.IncomeStabilityScore,
		ExpenseRegularityScore: analysis.ExpenseRegularityScore,
		SavingsRateBps:         analysis.SavingsRateBps,
		TransactionCount:       analysis.TransactionCount,
		UniqueIncomeSources:    analysis.UniqueIncomeSources,
		IncomeSources:          incomeSources,
		ExpenseCategories:      expenseCategories,
		SeasonalPattern:        seasonalPattern,
		RiskFlags:              riskFlags,
		CalculatedAt:           analysis.CalculatedAt,
		CreatedAt:              analysis.CreatedAt,
		UpdatedAt:              analysis.UpdatedAt,
	}
}

// toJSONField converts a map to *datatypes.JSON
func toJSONField(data interface{}) *datatypes.JSON {
	if data == nil {
		return nil
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	result := datatypes.JSON(jsonData)
	return &result
}
