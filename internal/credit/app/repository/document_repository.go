package repository

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	pkgErrors "github.com/Shamba-Records-Limited/Microvault/pkg/errors"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Common errors for DocumentRepository
var (
	ErrDocumentNotFound                = errors.New("no document found for user")
	ErrDocumentExpired                 = errors.New("document expired")
	ErrDocumentInvalid                 = errors.New("document invalid")
	ErrFailedToCreateDocument          = errors.New("failed to create document")
	ErrFailedToBatchCreateDocuments    = errors.New("failed to batch create documents")
	ErrFailedToGetDocument             = errors.New("failed to get document")
	ErrFailedToGetDocumentsByUserID    = errors.New("failed to get documents by user ID")
	ErrFailedToGetPendingDocuments     = errors.New("failed to get pending documents for user")
	ErrFailedToUpdateDocumentStatus    = errors.New("failed to update document status")
	ErrFailedToUpdateParsedData        = errors.New("failed to update parsed data")
	ErrFailedToRestoreDocument         = errors.New("failed to restore document")
	ErrFailedToDeleteDocument          = errors.New("failed to delete document")
	ErrFailedToDeleteDocumentsByUserID = errors.New("failed to delete documents by user ID")
)

// DocumentRepository defines the interface for document access
type DocumentRepository interface {
	// Create operations
	Create(ctx context.Context, doc *models.UserDocument) error
	BatchCreate(ctx context.Context, docs []*models.UserDocument) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.UserDocument, error)
	GetByUserID(ctx context.Context, userID string, docType string, limit, offset int) ([]*models.UserDocument, error)
	GetPendingDocuments(ctx context.Context, userID string, limit, offset int) ([]*models.UserDocument, error)

	// Update operations
	UpdateStatus(ctx context.Context, id string, status string, errorMsg *string) error
	UpdateParsedData(ctx context.Context, id string, parsedData *datatypes.JSON) error
	Restore(ctx context.Context, id string) error

	// Delete operations
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error
}

// DocumentRepository represents a repository for managing user documents.
type documentRepository struct {
	db *gorm.DB
}

// NewDocumentRepository creates a new instance of DocumentRepository.
func NewDocumentRepository(db *gorm.DB) (DocumentRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &documentRepository{db: db}, nil
}

// --- Create operations ---

// Create creates a new user document.
func (r *documentRepository) Create(ctx context.Context, doc *models.UserDocument) error {
	result := r.db.WithContext(ctx).Create(doc)
	if result.Error != nil {
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateDocument
	}
	return nil
}

// BatchCreate creates multiple user documents in a single batch.
func (r *documentRepository) BatchCreate(ctx context.Context, docs []*models.UserDocument) error {
	if len(docs) == 0 {
		return nil
	}

	result := r.db.WithContext(ctx).Create(docs)
	if result.Error != nil {
		log.Printf("BatchCreate: database error: %v", result.Error)
		return ErrFailedToBatchCreateDocuments
	}
	return nil
}

// --- Read operations ---

// GetByID retrieves a user document by its ID.
func (r *documentRepository) GetByID(ctx context.Context, id string) (*models.UserDocument, error) {
	var document models.UserDocument

	result := r.db.WithContext(ctx).Where("id = ?", id).First(&document)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrDocumentNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetDocument
	}
	return &document, nil
}

// GetByUserID retrieves user documents based on user ID, document type, limit, and offset.
func (r *documentRepository) GetByUserID(ctx context.Context, userID string, docType string, limit, offset int) ([]*models.UserDocument, error) {
	var documents []*models.UserDocument

	query := r.db.WithContext(ctx).Where("user_id = ?", userID)

	if docType != "" {
		query = query.Where("document_type = ?", docType)
	}

	result := query.
		Order("upload_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&documents)

	if result.Error != nil {
		log.Printf("GetByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetDocumentsByUserID
	}
	return documents, nil
}

// GetPendingDocuments retrieves pending documents for a user.
func (r *documentRepository) GetPendingDocuments(ctx context.Context, userID string, limit, offset int) ([]*models.UserDocument, error) {
	var documents []*models.UserDocument
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND processing_status = ?", userID, models.ProcessingStatusPending).
		Order("upload_date ASC").
		Limit(limit).
		Offset(offset).
		Find(&documents)
	if result.Error != nil {
		log.Printf("GetPendingDocuments: database error: %v", result.Error)
		return nil, ErrFailedToGetPendingDocuments
	}
	return documents, nil
}

// --- Update operations ---

// UpdateStatus updates the processing status of a document.
func (r *documentRepository) UpdateStatus(ctx context.Context, id string, status string, errorMsg *string) error {
	result := r.db.WithContext(ctx).
		Model(&models.UserDocument{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"processing_status": status,
			"error_message":     errorMsg,
			"updated_at":        time.Now(),
		})
	if result.RowsAffected == 0 {
		return ErrDocumentNotFound
	}
	if result.Error != nil {
		log.Printf("UpdateStatus: database error: %v", result.Error)
		return ErrFailedToUpdateDocumentStatus
	}
	return nil
}

// UpdateParsedData updates the parsed data of a document.
func (r *documentRepository) UpdateParsedData(ctx context.Context, id string, parsedData *datatypes.JSON) error {
	result := r.db.WithContext(ctx).
		Model(&models.UserDocument{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"parsed_data":       parsedData,
			"processing_status": models.ProcessingStatusCompleted,
			"updated_at":        time.Now(),
		})
	if result.RowsAffected == 0 {
		return ErrDocumentNotFound
	}
	if result.Error != nil {
		log.Printf("UpdateParsedData: database error: %v", result.Error)
		return ErrFailedToUpdateParsedData
	}
	return nil
}

// Restore restores a document by ID.
func (r *documentRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.UserDocument{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrDocumentNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreDocument
	}
	return nil
}

// --- Delete operations ---

// Delete deletes a document by its ID.
func (r *documentRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.UserDocument{})
	if result.RowsAffected == 0 {
		return ErrDocumentNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteDocument
	}
	return nil
}

// DeleteByUserID deletes all documents associated with a user.
func (r *documentRepository) DeleteByUserID(ctx context.Context, userID string) error {
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&models.UserDocument{})
	if result.Error != nil {
		log.Printf("DeleteByUserID: database error: %v", result.Error)
		return ErrFailedToDeleteDocumentsByUserID
	}
	return nil
}
