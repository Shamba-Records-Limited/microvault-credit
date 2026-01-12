package transactionextracted

import "errors"

// TransactionExtracted service specific errors
var (
	// Resource not found errors
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrDocumentNotFound    = errors.New("document not found")

	// Validation errors
	ErrInvalidInput     = errors.New("invalid input")
	ErrInvalidDateRange = errors.New("invalid date range")
	ErrInvalidAmount    = errors.New("invalid amount")
	ErrInvalidCategory  = errors.New("invalid category")
	ErrInvalidSource    = errors.New("invalid source")
)
