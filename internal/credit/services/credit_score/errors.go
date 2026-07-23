package creditscore

import "errors"

// CreditScore service specific errors
var (
	// Resource not found errors
	ErrCreditScoreNotFound = errors.New("credit score not found")

	// Business logic errors
	ErrCreditScoreExpired        = errors.New("credit score has expired")
	ErrInvalidCreditScore        = errors.New("invalid credit score")
	ErrScoreOutOfRange           = errors.New("score out of valid range")
	ErrCannotUpdateExpiredScore  = errors.New("cannot update expired credit score")
	ErrUserAlreadyHasCreditScore = errors.New("user already has a credit score")

	// Validation errors
	ErrInvalidInput        = errors.New("invalid input")
	ErrInvalidScore        = errors.New("invalid score value")
	ErrInvalidScoreVersion = errors.New("invalid score version")
)
