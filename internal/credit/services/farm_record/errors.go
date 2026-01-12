package farmrecord

import "errors"

// FarmRecord service specific errors
var (
	// Resource not found errors
	ErrFarmRecordNotFound = errors.New("farm record not found")

	// Business logic errors
	ErrInvalidFarmSize      = errors.New("invalid farm size")
	ErrInvalidCropType      = errors.New("invalid crop type")
	ErrInvalidOwnershipType = errors.New("invalid ownership type")

	// Validation errors
	ErrInvalidInput     = errors.New("invalid input")
	ErrInvalidLocation  = errors.New("invalid location")
	ErrInvalidDateRange = errors.New("invalid date range")
)
