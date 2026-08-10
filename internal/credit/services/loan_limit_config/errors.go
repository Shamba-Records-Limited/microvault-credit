package loanlimitconfig

import "errors"

// LoanLimitConfig service specific errors
var (
	// Resource not found errors
	ErrLoanLimitConfigNotFound = errors.New("loan limit config not found")

	// Conflict errors
	ErrRiskTierAlreadyExists = errors.New("risk tier already exists")

	// Business logic errors
	ErrConfigNotActive       = errors.New("loan limit config is not active")
	ErrConfigAlreadyActive   = errors.New("loan limit config is already active")
	ErrConfigAlreadyInactive = errors.New("loan limit config is already inactive")
	ErrInvalidAmountRange    = errors.New("max loan amount must be greater than min loan amount")

	// Validation errors
	ErrInvalidInput        = errors.New("invalid input")
	ErrInvalidRiskTier     = errors.New("invalid risk tier")
	ErrInvalidMinAmount    = errors.New("invalid minimum loan amount")
	ErrInvalidMaxAmount    = errors.New("invalid maximum loan amount")
	ErrInvalidInterestRate = errors.New("invalid interest rate")
)
