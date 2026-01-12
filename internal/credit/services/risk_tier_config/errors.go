package risktierconfig

import "errors"

// RiskTierConfig service specific errors
var (
	// Resource not found errors
	ErrRiskTierConfigNotFound = errors.New("risk tier config not found")

	// Conflict errors
	ErrTierNameAlreadyExists  = errors.New("tier name already exists")
	ErrTierOrderAlreadyExists = errors.New("tier order already exists")

	// Business logic errors
	ErrConfigNotActive       = errors.New("risk tier config is not active")
	ErrTierAlreadyActive     = errors.New("risk tier config is already active")
	ErrTierAlreadyInactive   = errors.New("risk tier config is already inactive")
	ErrInvalidScoreRange     = errors.New("max score must be greater than min score")
	ErrOverlappingScoreRange = errors.New("score range overlaps with existing tier")

	// Validation errors
	ErrInvalidInput     = errors.New("invalid input")
	ErrInvalidTierName  = errors.New("invalid tier name")
	ErrInvalidMinScore  = errors.New("invalid minimum score")
	ErrInvalidMaxScore  = errors.New("invalid maximum score")
	ErrInvalidTierOrder = errors.New("invalid tier order")
)
