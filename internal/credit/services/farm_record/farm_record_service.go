package farmrecord

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

// Service defines the interface for farm record business logic operations
type Service interface {
	// Farm record management
	Create(ctx context.Context, req CreateFarmRecordRequest) (*FarmRecordResponse, error)
	BatchCreate(ctx context.Context, reqs []CreateFarmRecordRequest) ([]*FarmRecordResponse, error)
	GetByID(ctx context.Context, id string) (*FarmRecordResponse, error)
	GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[FarmRecordResponse], error)
	GetVerified(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[FarmRecordResponse], error)
	Update(ctx context.Context, id string, req UpdateFarmRecordRequest) (*FarmRecordResponse, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error

	// Verification
	Verify(ctx context.Context, id string, verifiedBy string) (*FarmRecordResponse, error)

	// Aggregations
	GetSummaryByUser(ctx context.Context, userID string, startDate, endDate time.Time) (*FarmRecordSummaryResponse, error)
}

// service implements the Service interface
type service struct {
	repo repository.FarmRecordRepository
}

// NewService creates a new farm record service instance
func NewService(repo repository.FarmRecordRepository) Service {
	return &service{
		repo: repo,
	}
}

// Create creates a new farm record with business validation
func (s *service) Create(ctx context.Context, req CreateFarmRecordRequest) (*FarmRecordResponse, error) {
	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Create farm record model
	// Map request fields to metadata JSON since the model schema doesn't match the request
	metadata := map[string]interface{}{
		"farm_name":             req.FarmName,
		"location":              req.Location,
		"size_hectares":         req.SizeHectares,
		"ownership_type":        req.OwnershipType,
		"secondary_crops":       req.SecondaryCrops,
		"irrigation_type":       req.IrrigationType,
		"annual_yield_estimate": req.AnnualYieldEstimate,
		"last_harvest_date":     req.LastHarvestDate,
	}

	record := &models.FarmRecord{
		UserID:     req.UserID,
		RecordType: "farm_property",
		RecordDate: time.Now(),
		CropType:   req.PrimaryCrop,
		Verified:   req.IsVerified,
		Metadata:   toJSONField(metadata),
	}

	// Create farm record in database
	if err := s.repo.Create(ctx, record); err != nil {
		log.Printf("Create: failed to create farm record: %v", err)
		return nil, err
	}

	return toFarmRecordResponse(record), nil
}

// BatchCreate creates multiple farm records with business validation
func (s *service) BatchCreate(ctx context.Context, reqs []CreateFarmRecordRequest) ([]*FarmRecordResponse, error) {
	if len(reqs) == 0 {
		return []*FarmRecordResponse{}, nil
	}

	// Validate all requests
	for i, req := range reqs {
		if err := s.validateCreateRequest(req); err != nil {
			log.Printf("BatchCreate: request %d invalid: %v", i, err)
			return nil, err
		}
	}

	// Convert to models
	records := make([]*models.FarmRecord, len(reqs))
	for i, req := range reqs {
		// Map request fields to metadata JSON
		metadata := map[string]interface{}{
			"farm_name":             req.FarmName,
			"location":              req.Location,
			"size_hectares":         req.SizeHectares,
			"ownership_type":        req.OwnershipType,
			"secondary_crops":       req.SecondaryCrops,
			"irrigation_type":       req.IrrigationType,
			"annual_yield_estimate": req.AnnualYieldEstimate,
			"last_harvest_date":     req.LastHarvestDate,
		}

		records[i] = &models.FarmRecord{
			UserID:     req.UserID,
			RecordType: "farm_property",
			RecordDate: time.Now(),
			CropType:   req.PrimaryCrop,
			Verified:   req.IsVerified,
			Metadata:   toJSONField(metadata),
		}
	}

	// Batch create in single transaction
	if err := s.repo.BatchCreate(ctx, records); err != nil {
		log.Printf("BatchCreate: failed to create %d farm records: %v", len(records), err)
		return nil, err
	}

	log.Printf("BatchCreate: successfully created %d farm records", len(records))

	// Convert to responses
	responses := make([]*FarmRecordResponse, len(records))
	for i, record := range records {
		responses[i] = toFarmRecordResponse(record)
	}

	return responses, nil
}

// GetByID retrieves a farm record by ID
func (s *service) GetByID(ctx context.Context, id string) (*FarmRecordResponse, error) {
	record, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrFarmRecordNotFound) {
			return nil, ErrFarmRecordNotFound
		}
		log.Printf("GetByID: failed to get farm record: %v", err)
		return nil, err
	}

	return toFarmRecordResponse(record), nil
}

// GetByUserID retrieves farm records by user ID with pagination
func (s *service) GetByUserID(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[FarmRecordResponse], error) {
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

	records, err := s.repo.GetByUserID(ctx, userID, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetByUserID: failed to get farm records: %v", err)
		return nil, err
	}

	responses := make([]FarmRecordResponse, len(records))
	for i, record := range records {
		responses[i] = *toFarmRecordResponse(record)
	}

	return &services.PaginatedResponse[FarmRecordResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// GetVerified retrieves verified farm records by user ID with pagination
func (s *service) GetVerified(ctx context.Context, userID string, pagination services.Pagination) (*services.PaginatedResponse[FarmRecordResponse], error) {
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

	records, err := s.repo.GetVerifiedByUserID(ctx, userID, pagination.PageSize, offset)
	if err != nil {
		log.Printf("GetVerified: failed to get verified farm records: %v", err)
		return nil, err
	}

	responses := make([]FarmRecordResponse, len(records))
	for i, record := range records {
		responses[i] = *toFarmRecordResponse(record)
	}

	return &services.PaginatedResponse[FarmRecordResponse]{
		Data:     responses,
		Page:     pagination.Page,
		PageSize: pagination.PageSize,
	}, nil
}

// Update updates farm record information
func (s *service) Update(ctx context.Context, id string, req UpdateFarmRecordRequest) (*FarmRecordResponse, error) {
	record, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrFarmRecordNotFound) {
			return nil, ErrFarmRecordNotFound
		}
		log.Printf("Update: failed to get farm record: %v", err)
		return nil, err
	}

	// Extract existing metadata
	var metadata map[string]interface{}
	if record.Metadata != nil {
		if err := json.Unmarshal(*record.Metadata, &metadata); err != nil {
			log.Printf("Warning: failed to unmarshal metadata: %v", err)
		}
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	// Update metadata fields
	if req.FarmName != nil {
		metadata["farm_name"] = *req.FarmName
	}
	if req.Location != nil {
		metadata["location"] = *req.Location
	}
	if req.SizeHectares != nil {
		metadata["size_hectares"] = *req.SizeHectares
	}
	if req.OwnershipType != nil {
		metadata["ownership_type"] = *req.OwnershipType
	}
	if req.SecondaryCrops != nil {
		metadata["secondary_crops"] = req.SecondaryCrops
	}
	if req.IrrigationType != nil {
		metadata["irrigation_type"] = *req.IrrigationType
	}
	if req.AnnualYieldEstimate != nil {
		metadata["annual_yield_estimate"] = *req.AnnualYieldEstimate
	}
	if req.LastHarvestDate != nil {
		metadata["last_harvest_date"] = *req.LastHarvestDate
	}

	// Update model fields
	if req.PrimaryCrop != nil {
		record.CropType = req.PrimaryCrop
	}
	if req.IsVerified != nil {
		record.Verified = *req.IsVerified
	}

	// Save updated metadata
	record.Metadata = toJSONField(metadata)

	// Update in database
	if err := s.repo.Update(ctx, record); err != nil {
		log.Printf("Update: failed to update farm record: %v", err)
		return nil, err
	}

	return toFarmRecordResponse(record), nil
}

// Delete soft deletes a farm record
func (s *service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrFarmRecordNotFound) {
			return ErrFarmRecordNotFound
		}
		log.Printf("Delete: failed to delete farm record: %v", err)
		return err
	}

	return nil
}

// Restore restores a soft-deleted farm record
func (s *service) Restore(ctx context.Context, id string) error {
	if err := s.repo.Restore(ctx, id); err != nil {
		if errors.Is(err, repository.ErrFarmRecordNotFound) {
			return ErrFarmRecordNotFound
		}
		log.Printf("Restore: failed to restore farm record: %v", err)
		return err
	}

	return nil
}

// Verify marks a farm record as verified
func (s *service) Verify(ctx context.Context, id string, verifiedBy string) (*FarmRecordResponse, error) {
	record, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrFarmRecordNotFound) {
			return nil, ErrFarmRecordNotFound
		}
		log.Printf("Verify: failed to get farm record: %v", err)
		return nil, err
	}

	// Update verification status
	record.Verified = true

	// Store verification metadata
	var metadata map[string]interface{}
	if record.Metadata != nil {
		if err := json.Unmarshal(*record.Metadata, &metadata); err != nil {
			log.Printf("Warning: failed to unmarshal metadata: %v", err)
		}
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["verified_at"] = time.Now()
	metadata["verified_by"] = verifiedBy
	record.Metadata = toJSONField(metadata)

	if err := s.repo.Update(ctx, record); err != nil {
		log.Printf("Verify: failed to update farm record: %v", err)
		return nil, err
	}

	return toFarmRecordResponse(record), nil
}

// GetSummaryByUser retrieves comprehensive farm record summary for a user within a date range
func (s *service) GetSummaryByUser(ctx context.Context, userID string, startDate, endDate time.Time) (*FarmRecordSummaryResponse, error) {
	// Validate inputs
	if userID == "" {
		return nil, ErrInvalidInput
	}
	if startDate.IsZero() || endDate.IsZero() {
		return nil, ErrInvalidDateRange
	}
	if endDate.Before(startDate) {
		return nil, ErrInvalidDateRange
	}

	// Get summary from repository
	summary, err := s.repo.GetSummaryByUser(ctx, userID, startDate, endDate)
	if err != nil {
		log.Printf("GetSummaryByUser: failed to get summary for user %s: %v", userID, err)
		return nil, err
	}

	return &FarmRecordSummaryResponse{
		UserID:           summary.UserID,
		StartDate:        summary.DateRange.Start,
		EndDate:          summary.DateRange.End,
		TotalRecords:     summary.TotalRecords,
		VerifiedRecords:  summary.VerifiedRecords,
		TotalSalesAmount: summary.TotalSalesAmount,
		TotalExpenses:    summary.TotalExpenses,
		TotalInputCosts:  summary.TotalInputCosts,
		CropTypes:        summary.CropTypes,
		RecordsByType:    summary.RecordsByType,
		MonthlySales:     summary.MonthlySales,
	}, nil
}

// --- Helper functions ---

// validateCreateRequest validates the create farm record request
func (s *service) validateCreateRequest(req CreateFarmRecordRequest) error {
	if req.UserID == "" {
		return ErrInvalidInput
	}

	if req.FarmName == "" {
		return ErrInvalidInput
	}

	if req.SizeHectares != nil && *req.SizeHectares < 0 {
		return ErrInvalidFarmSize
	}

	return nil
}

// toFarmRecordResponse converts a farm record model to response DTO
func toFarmRecordResponse(record *models.FarmRecord) *FarmRecordResponse {
	// Extract metadata fields
	var metadata map[string]interface{}
	if record.Metadata != nil {
		if err := json.Unmarshal(*record.Metadata, &metadata); err != nil {
			log.Printf("Warning: failed to unmarshal metadata: %v", err)
		}
	}

	response := &FarmRecordResponse{
		ID:         record.ID,
		UserID:     record.UserID,
		IsVerified: record.Verified,
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	}

	// Extract metadata fields if present
	if metadata != nil {
		if farmName, ok := metadata["farm_name"].(string); ok {
			response.FarmName = farmName
		}
		if location, ok := metadata["location"].(string); ok {
			response.Location = &location
		}
		if sizeHectares, ok := metadata["size_hectares"].(float64); ok {
			response.SizeHectares = &sizeHectares
		}
		if ownershipType, ok := metadata["ownership_type"].(string); ok {
			response.OwnershipType = &ownershipType
		}
		if irrigationType, ok := metadata["irrigation_type"].(string); ok {
			response.IrrigationType = &irrigationType
		}
		if annualYieldEstimate, ok := metadata["annual_yield_estimate"].(float64); ok {
			estimate := int64(annualYieldEstimate)
			response.AnnualYieldEstimate = &estimate
		}
	}

	// Set primary crop from model
	response.PrimaryCrop = record.CropType

	return response
}

// toJSONField converts data to *datatypes.JSON
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
