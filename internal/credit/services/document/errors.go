package document

import "errors"

// Document service specific errors
var (
	// Resource not found errors
	ErrDocumentNotFound = errors.New("document not found")

	// Business logic errors
	ErrDocumentAlreadyProcessed = errors.New("document already processed")
	ErrDocumentProcessingFailed = errors.New("document processing failed")
	ErrInvalidDocumentType      = errors.New("invalid document type")
	ErrInvalidDocumentStatus    = errors.New("invalid document status")
	ErrDocumentExpired          = errors.New("document has expired")

	// Validation errors
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidFileURL     = errors.New("invalid file URL")
	ErrInvalidContentHash = errors.New("invalid content hash")
)
