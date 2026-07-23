package creditfactor

import "errors"

// CreditFactor service specific errors
var (
	// Resource not found errors
	ErrCreditFactorNotFound = errors.New("credit factor not found")

	// Business logic errors
	ErrInvalidCreditFactor = errors.New("invalid credit factor: values out of acceptable range")
	ErrFactorAlreadyExists = errors.New("credit factor already exists for this user and score")

	// Validation errors
	ErrInvalidInput        = errors.New("invalid input")
	ErrInvalidFactorValue  = errors.New("invalid factor value")
	ErrInvalidFactorWeight = errors.New("invalid factor weight")
	ErrInvalidFactorName   = errors.New("invalid factor name")
)
