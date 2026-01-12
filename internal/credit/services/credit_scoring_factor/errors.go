package creditscoringfactor

import "errors"

// CreditScoringFactor service specific errors
var (
	// Resource not found errors
	ErrCreditScoringFactorNotFound = errors.New("credit scoring factor not found")

	// Conflict errors
	ErrFactorNameAlreadyExists = errors.New("factor name already exists")

	// Business logic errors
	ErrFactorNotActive      = errors.New("credit scoring factor is not active")
	ErrFactorAlreadyActive  = errors.New("credit scoring factor is already active")
	ErrFactorAlreadyInactive = errors.New("credit scoring factor is already inactive")

	// Validation errors
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidFactorName  = errors.New("invalid factor name")
	ErrInvalidWeight      = errors.New("invalid weight")
)
