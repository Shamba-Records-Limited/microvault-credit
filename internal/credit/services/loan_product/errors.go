package loanproduct

import "errors"

// LoanProduct service specific errors
var (
	// Resource not found errors
	ErrLoanProductNotFound = errors.New("loan product not found")

	// Conflict errors
	ErrProductNameAlreadyExists = errors.New("loan product name already exists")

	// Business logic errors
	ErrProductNotActive     = errors.New("loan product is not active")
	ErrInvalidAmountRange   = errors.New("max amount must be greater than min amount")
	ErrInvalidDurationRange = errors.New("max duration must be greater than min duration")

	// Validation errors
	ErrInvalidInput        = errors.New("invalid input")
	ErrInvalidProductName  = errors.New("invalid product name")
	ErrInvalidInterestRate = errors.New("invalid interest rate")
	ErrInvalidInterestType = errors.New("invalid interest type")
	ErrInvalidMinAmount    = errors.New("invalid minimum amount")
	ErrInvalidMaxAmount    = errors.New("invalid maximum amount")
	ErrInvalidMinDuration  = errors.New("invalid minimum duration")
	ErrInvalidMaxDuration  = errors.New("invalid maximum duration")
)
