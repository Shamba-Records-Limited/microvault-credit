package globallendinglimit

import "errors"

// GlobalLendingLimit service specific errors
var (
	// Resource not found errors
	ErrGlobalLendingLimitNotFound = errors.New("global lending limit not found")

	// Conflict errors
	ErrConfigKeyAlreadyExists = errors.New("config key already exists")

	// Business logic errors
	ErrConfigNotActive       = errors.New("global lending limit config is not active")
	ErrConfigAlreadyActive   = errors.New("global lending limit config is already active")
	ErrConfigAlreadyInactive = errors.New("global lending limit config is already inactive")

	// Validation errors
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidConfigKey   = errors.New("invalid config key")
	ErrInvalidConfigValue = errors.New("invalid config value")
	ErrInvalidValueType   = errors.New("invalid value type")
)
