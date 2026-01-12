package cashflowanalysis

import "errors"

// CashflowAnalysis service specific errors
var (
	// Resource not found errors
	ErrCashflowAnalysisNotFound = errors.New("cashflow analysis not found")

	// Business logic errors
	ErrInvalidCashflowAnalysis = errors.New("invalid cashflow analysis data")
	ErrAnalysisStale           = errors.New("cashflow analysis is stale")
	ErrInvalidAnalysisPeriod   = errors.New("invalid analysis period")
	ErrNegativeIncome          = errors.New("total income cannot be negative")
	ErrNegativeExpenses        = errors.New("total expenses cannot be negative")

	// Validation errors
	ErrInvalidInput     = errors.New("invalid input")
	ErrInvalidDateRange = errors.New("invalid date range")
)
