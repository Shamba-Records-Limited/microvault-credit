package cashflowanalysis

import "time"

// CreateCashflowAnalysisRequest represents the request to create a new cashflow analysis
type CreateCashflowAnalysisRequest struct {
	UserID                string            `json:"user_id" validate:"required"`
	AnalysisPeriodStart   time.Time         `json:"analysis_period_start" validate:"required"`
	AnalysisPeriodEnd     time.Time         `json:"analysis_period_end" validate:"required"`
	TotalIncome           int64             `json:"total_income" validate:"gte=0"`
	TotalExpenses         int64             `json:"total_expenses" validate:"gte=0"`
	IncomeStabilityScore  int               `json:"income_stability_score"`
	ExpenseRegularityScore int              `json:"expense_regularity_score"`
	TransactionCount      int               `json:"transaction_count"`
	UniqueIncomeSources   int               `json:"unique_income_sources"`
	IncomeSources         map[string]int64  `json:"income_sources,omitempty"`
	ExpenseCategories     map[string]int64  `json:"expense_categories,omitempty"`
	SeasonalPattern       *string           `json:"seasonal_pattern,omitempty"`
	RiskFlags             []string          `json:"risk_flags,omitempty"`
}

// UpdateCashflowAnalysisRequest represents the request to update cashflow analysis information
type UpdateCashflowAnalysisRequest struct {
	TotalIncome            *int64            `json:"total_income,omitempty"`
	TotalExpenses          *int64            `json:"total_expenses,omitempty"`
	IncomeStabilityScore   *int              `json:"income_stability_score,omitempty"`
	ExpenseRegularityScore *int              `json:"expense_regularity_score,omitempty"`
	TransactionCount       *int              `json:"transaction_count,omitempty"`
	UniqueIncomeSources    *int              `json:"unique_income_sources,omitempty"`
	IncomeSources          map[string]int64  `json:"income_sources,omitempty"`
	ExpenseCategories      map[string]int64  `json:"expense_categories,omitempty"`
	SeasonalPattern        *string           `json:"seasonal_pattern,omitempty"`
	RiskFlags              []string          `json:"risk_flags,omitempty"`
}

// CashflowAnalysisResponse represents the response containing cashflow analysis information
type CashflowAnalysisResponse struct {
	ID                     string           `json:"id"`
	UserID                 string           `json:"user_id"`
	AnalysisPeriodStart    time.Time        `json:"analysis_period_start"`
	AnalysisPeriodEnd      time.Time        `json:"analysis_period_end"`
	TotalIncome            int64            `json:"total_income"`
	TotalExpenses          int64            `json:"total_expenses"`
	NetCashflow            int64            `json:"net_cashflow"`
	AvgMonthlyIncome       int64            `json:"avg_monthly_income"`
	AvgMonthlyExpenses     int64            `json:"avg_monthly_expenses"`
	IncomeStabilityScore   int              `json:"income_stability_score"`
	ExpenseRegularityScore int              `json:"expense_regularity_score"`
	SavingsRateBps         int32            `json:"savings_rate_bps"`
	TransactionCount       int              `json:"transaction_count"`
	UniqueIncomeSources    int              `json:"unique_income_sources"`
	IncomeSources          map[string]int64 `json:"income_sources,omitempty"`
	ExpenseCategories      map[string]int64 `json:"expense_categories,omitempty"`
	SeasonalPattern        *string          `json:"seasonal_pattern,omitempty"`
	RiskFlags              []string         `json:"risk_flags,omitempty"`
	CalculatedAt           time.Time        `json:"calculated_at"`
	CreatedAt              time.Time        `json:"created_at"`
	UpdatedAt              time.Time        `json:"updated_at"`
}

// CashflowComparisonResponse represents a comparison between two cashflow analyses
type CashflowComparisonResponse struct {
	Current           *CashflowAnalysisResponse `json:"current"`
	Previous          *CashflowAnalysisResponse `json:"previous,omitempty"`
	IncomeChange      int64                     `json:"income_change"`
	ExpenseChange     int64                     `json:"expense_change"`
	NetCashflowChange int64                     `json:"net_cashflow_change"`
}
