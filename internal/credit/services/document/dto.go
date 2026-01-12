package document

import "time"

// CreateDocumentRequest represents the request to create a new document
type CreateDocumentRequest struct {
	UserID       string  `json:"user_id" validate:"required"`
	DocumentType string  `json:"document_type" validate:"required"`
	FileName     string  `json:"file_name" validate:"required"`
	FileURL      string  `json:"file_url" validate:"required"`
	FileSize     *int64  `json:"file_size,omitempty"`
	MimeType     *string `json:"mime_type,omitempty"`
	ContentHash  *string `json:"content_hash,omitempty"`
}

// UpdateDocumentRequest represents the request to update document information
type UpdateDocumentRequest struct {
	Status         *string                `json:"status,omitempty"`
	ProcessedAt    *time.Time             `json:"processed_at,omitempty"`
	ExtractedData  map[string]interface{} `json:"extracted_data,omitempty"`
	ExtractionNote *string                `json:"extraction_note,omitempty"`
	VerifiedAt     *time.Time             `json:"verified_at,omitempty"`
	VerifiedBy     *string                `json:"verified_by,omitempty"`
	ExpiresAt      *time.Time             `json:"expires_at,omitempty"`
}

// DocumentResponse represents the response containing document information
type DocumentResponse struct {
	ID             string                 `json:"id"`
	UserID         string                 `json:"user_id"`
	DocumentType   string                 `json:"document_type"`
	FileName       string                 `json:"file_name"`
	FileURL        string                 `json:"file_url"`
	FileSize       *int64                 `json:"file_size,omitempty"`
	MimeType       *string                `json:"mime_type,omitempty"`
	ContentHash    *string                `json:"content_hash,omitempty"`
	Status         string                 `json:"status"`
	ProcessedAt    *time.Time             `json:"processed_at,omitempty"`
	ExtractedData  map[string]interface{} `json:"extracted_data,omitempty"`
	ExtractionNote *string                `json:"extraction_note,omitempty"`
	VerifiedAt     *time.Time             `json:"verified_at,omitempty"`
	VerifiedBy     *string                `json:"verified_by,omitempty"`
	ExpiresAt      *time.Time             `json:"expires_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// DocumentFilters represents filters for listing documents
type DocumentFilters struct {
	UserID       string `json:"user_id,omitempty"`
	DocumentType string `json:"document_type,omitempty"`
	Status       string `json:"status,omitempty"`
}
