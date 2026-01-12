package creditconfigauditlog

import "errors"

// CreditConfigAuditLog service specific errors
var (
	// Resource not found errors
	ErrCreditConfigAuditLogNotFound = errors.New("credit config audit log not found")

	// Validation errors
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidConfigTable = errors.New("invalid config table")
	ErrInvalidConfigID    = errors.New("invalid config ID")
	ErrInvalidAction      = errors.New("invalid action")
	ErrInvalidDateRange   = errors.New("invalid date range")
)
