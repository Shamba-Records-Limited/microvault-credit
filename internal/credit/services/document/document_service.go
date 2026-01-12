package document

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
	"gorm.io/datatypes"
)

// Service defines the interface for document business logic operations
type Service interface {
	// Document management
	Create(ctx context.Context, req CreateDocumentRequest) (*DocumentResponse, error)
	GetByID(ctx context.Context, id string) (*DocumentResponse, error)
	GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[DocumentResponse], error)
	GetByUserIDAndType(ctx context.Context, userID, documentType string, pagination services.Pagination) (*services.PaginatedResponse[DocumentResponse], error)
	GetPendingProcessing(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[DocumentResponse], error)
	Update(ctx context.Context, id string, req UpdateDocumentRequest) (*DocumentResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error

	// Document processing
	MarkAsProcessed(ctx context.Context, id string, extractedData map[string]interface{}, note *string) (*DocumentResponse, error)
	MarkAsVerified(ctx context.Context, id string, verifiedBy string) (*DocumentResponse, error)
	MarkAsFailed(ctx context.Context, id string, note string) (*DocumentResponse, error)
}

// service implements the Service interface
type service struct {
	repo repository.DocumentRepository
}

// NewService creates a new document service instance
func NewService(repo repository.DocumentRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new document with business validation
func (s *service) Create(ctx context.Context, req CreateDocumentRequest) (*DocumentResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Create document model
	fileSize := 0
	if req.FileSize != nil {
		fileSize = int(*req.FileSize)
	}
	mimeType := "application/octet-stream"
	if req.MimeType != nil {
		mimeType = *req.MimeType
	}

	doc := &models.UserDocument{
		UserID:           req.UserID,
		DocumentType:     req.DocumentType,
		FileName:         req.FileName,
		StoragePath:      req.FileURL,
		FileSize:         fileSize,
		MimeType:         mimeType,
		UploadDate:       time.Now(),
		ProcessingStatus: models.ProcessingStatusPending,
	}

	// Create document in database
	if err := s.repo.Create(ctx, doc); err != nil {
		log.Printf("Create: failed to create document: %v", err)
		return nil, err
	}

	return toDocumentResponse(doc), nil
}

// GetByID retrieves a document by ID
func (s *service) GetByID(ctx context.Context, id string) (*DocumentResponse, error) {
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrDocumentNotFound) {
			return nil, ErrDocumentNotFound
		}
		log.Printf("GetByID: failed to get document: %v", err)
		return nil, err
	}

	return toDocumentResponse(doc), nil
}

// GetByUserID retrieves documents by user ID with pagination
func (s *service) GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[DocumentResponse], error) {
	if pagination.Page <= 0 {
		pagination.Page = 1
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	if pagination.PageSize > 100 {
		pagination.PageSize = 100
	}

	offset := (pagination.Page - 1) * pagination.PageSize

	docs, err := s.repo.GetByUserID(ctx, userID, "", pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByUserID: failed to get documents: %v", err)
		return nil, err
	}

	responses := make([]DocumentResponse, len(docs))
	for i, doc := range docs {
		responses[i] = *toDocumentResponse(doc)
	}

	return &services.PaginatedResponse[DocumentResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetByUserIDAndType retrieves documents by user ID and type with pagination
func (s *service) GetByUserIDAndType(ctx context.Context, userID, documentType string, pagination services.Pagination) (*services.PaginatedResponse[DocumentResponse], error) {
	if pagination.Page <= 0 {
		pagination.Page = 1
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	if pagination.PageSize > 100 {
		pagination.PageSize = 100
	}

	offset := (pagination.Page - 1) * pagination.PageSize

	docs, err := s.repo.GetByUserID(ctx, userID, documentType, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByUserIDAndType: failed to get documents: %v", err)
		return nil, err
	}

	responses := make([]DocumentResponse, len(docs))
	for i, doc := range docs {
		responses[i] = *toDocumentResponse(doc)
	}

	return &services.PaginatedResponse[DocumentResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetPendingProcessing retrieves documents pending processing with pagination
func (s *service) GetPendingProcessing(ctx context.Context, pagination services.Pagination) (*services.PaginatedResponse[DocumentResponse], error) {
	if pagination.Page <= 0 {
		pagination.Page = 1
	}
	if pagination.PageSize <= 0 {
		pagination.PageSize = 10
	}
	if pagination.PageSize > 100 {
		pagination.PageSize = 100
	}

	offset := (pagination.Page - 1) * pagination.PageSize

	docs, err := s.repo.GetPendingDocuments(ctx, "", pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetPendingProcessing: failed to get documents: %v", err)
		return nil, err
	}

	responses := make([]DocumentResponse, len(docs))
	for i, doc := range docs {
		responses[i] = *toDocumentResponse(doc)
	}

	return &services.PaginatedResponse[DocumentResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// Update updates document information
func (s *service) Update(ctx context.Context, id string, req UpdateDocumentRequest) (*DocumentResponse, error) {
	// Update status if provided
	if req.Status != nil {
		errorMsg := req.ExtractionNote
		if err := s.repo.UpdateStatus(ctx, id, *req.Status, errorMsg); err != nil {
			if errors.Is(err, repository.ErrDocumentNotFound) {
				return nil, ErrDocumentNotFound
			}
			log.Printf("Update: failed to update status: %v", err)
			return nil, err
		}
	}

	// Update parsed data if provided
	if req.ExtractedData != nil {
		parsedData := toJSONField(req.ExtractedData)
		if err := s.repo.UpdateParsedData(ctx, id, parsedData); err != nil {
			if errors.Is(err, repository.ErrDocumentNotFound) {
				return nil, ErrDocumentNotFound
			}
			log.Printf("Update: failed to update parsed data: %v", err)
			return nil, err
		}
	}

	// Fetch updated document
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrDocumentNotFound) {
			return nil, ErrDocumentNotFound
		}
		log.Printf("Update: failed to get document: %v", err)
		return nil, err
	}

	return toDocumentResponse(doc), nil
}

// Delete soft deletes a document
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrDocumentNotFound) {
			return ErrDocumentNotFound
		}
		log.Printf("Delete: failed to delete document: %v", err)
		return err
	}

	return nil
}

// Restore restores a soft-deleted document
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrDocumentNotFound) {
			return ErrDocumentNotFound
		}
		log.Printf("Restore: failed to restore document: %v", err)
		return err
	}

	return nil
}

// MarkAsProcessed marks a document as processed with extracted data
func (s *service) MarkAsProcessed(ctx context.Context, id string, extractedData map[string]interface{}, note *string) (*DocumentResponse, error) {
	// Update status to completed
	if err := s.repo.UpdateStatus(ctx, id, models.ProcessingStatusCompleted, note); err != nil {
		if errors.Is(err, repository.ErrDocumentNotFound) {
			return nil, ErrDocumentNotFound
		}
		log.Printf("MarkAsProcessed: failed to update status: %v", err)
		return nil, err
	}

	// Update parsed data
	parsedData := toJSONField(extractedData)
	if err := s.repo.UpdateParsedData(ctx, id, parsedData); err != nil {
		log.Printf("MarkAsProcessed: failed to update parsed data: %v", err)
		return nil, err
	}

	// Refresh document
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.Printf("MarkAsProcessed: failed to refresh document: %v", err)
		return nil, err
	}

	return toDocumentResponse(doc), nil
}

// MarkAsVerified marks a document as verified
func (s *service) MarkAsVerified(ctx context.Context, id string, verifiedBy string) (*DocumentResponse, error) {
	// Update status to verified
	// Note: verifiedBy information is not stored in the current model
	if err := s.repo.UpdateStatus(ctx, id, models.ProcessingStatusVerified, nil); err != nil {
		if errors.Is(err, repository.ErrDocumentNotFound) {
			return nil, ErrDocumentNotFound
		}
		log.Printf("MarkAsVerified: failed to update status: %v", err)
		return nil, err
	}

	// Refresh document
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.Printf("MarkAsVerified: failed to refresh document: %v", err)
		return nil, err
	}

	return toDocumentResponse(doc), nil
}

// MarkAsFailed marks a document processing as failed
func (s *service) MarkAsFailed(ctx context.Context, id string, note string) (*DocumentResponse, error) {
	// Update status to failed with error message
	if err := s.repo.UpdateStatus(ctx, id, models.ProcessingStatusFailed, &note); err != nil {
		if errors.Is(err, repository.ErrDocumentNotFound) {
			return nil, ErrDocumentNotFound
		}
		log.Printf("MarkAsFailed: failed to update status: %v", err)
		return nil, err
	}

	// Refresh document
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		log.Printf("MarkAsFailed: failed to refresh document: %v", err)
		return nil, err
	}

	return toDocumentResponse(doc), nil
}

// --- Helper functions ---

// validateCreateRequest validates the create document request
func (s *service) validateCreateRequest(req CreateDocumentRequest) error {
	if req.UserID == "" {
		return ErrInvalidInput
	}

	if req.DocumentType == "" {
		return ErrInvalidDocumentType
	}

	if req.FileName == "" {
		return ErrInvalidInput
	}

	if req.FileURL == "" {
		return ErrInvalidFileURL
	}

	return nil
}

// toDocumentResponse converts a document model to response DTO
func toDocumentResponse(doc *models.UserDocument) *DocumentResponse {
	fileSize := int64(doc.FileSize)
	var extractedData map[string]interface{}
	if doc.ParsedData != nil {
		if err := json.Unmarshal(*doc.ParsedData, &extractedData); err != nil {
			log.Printf("Warning: failed to unmarshal parsed data: %v", err)
		}
	}

	return &DocumentResponse{
		ID:             doc.ID,
		UserID:         doc.UserID,
		DocumentType:   doc.DocumentType,
		FileName:       doc.FileName,
		FileURL:        doc.StoragePath,
		FileSize:       &fileSize,
		MimeType:       &doc.MimeType,
		Status:         doc.ProcessingStatus,
		ExtractedData:  extractedData,
		ExtractionNote: doc.ErrorMessage,
		CreatedAt:      doc.CreatedAt,
		UpdatedAt:      doc.UpdatedAt,
	}
}

// toJSONField converts a map to *datatypes.JSON
func toJSONField(data interface{}) *datatypes.JSON {
	if data == nil {
		return nil
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	result := datatypes.JSON(jsonData)
	return &result
}
