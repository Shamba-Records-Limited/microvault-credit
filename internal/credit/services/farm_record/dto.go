package farmrecord

import "time"

// CreateFarmRecordRequest represents the request to create a new farm record
type CreateFarmRecordRequest struct {
	UserID              string     `json:"user_id" validate:"required"`
	FarmName            string     `json:"farm_name" validate:"required"`
	Location            *string    `json:"location,omitempty"`
	SizeHectares        *float64   `json:"size_hectares,omitempty"`
	OwnershipType       *string    `json:"ownership_type,omitempty"`
	PrimaryCrop         *string    `json:"primary_crop,omitempty"`
	SecondaryCrops      []string   `json:"secondary_crops,omitempty"`
	IrrigationType      *string    `json:"irrigation_type,omitempty"`
	AnnualYieldEstimate *int64     `json:"annual_yield_estimate,omitempty"`
	LastHarvestDate     *time.Time `json:"last_harvest_date,omitempty"`
	IsVerified          bool       `json:"is_verified"`
}

// UpdateFarmRecordRequest represents the request to update farm record information
type UpdateFarmRecordRequest struct {
	FarmName            *string    `json:"farm_name,omitempty"`
	Location            *string    `json:"location,omitempty"`
	SizeHectares        *float64   `json:"size_hectares,omitempty"`
	OwnershipType       *string    `json:"ownership_type,omitempty"`
	PrimaryCrop         *string    `json:"primary_crop,omitempty"`
	SecondaryCrops      []string   `json:"secondary_crops,omitempty"`
	IrrigationType      *string    `json:"irrigation_type,omitempty"`
	AnnualYieldEstimate *int64     `json:"annual_yield_estimate,omitempty"`
	LastHarvestDate     *time.Time `json:"last_harvest_date,omitempty"`
	IsVerified          *bool      `json:"is_verified,omitempty"`
	VerifiedAt          *time.Time `json:"verified_at,omitempty"`
	VerifiedBy          *string    `json:"verified_by,omitempty"`
}

// FarmRecordResponse represents the response containing farm record information
type FarmRecordResponse struct {
	ID                  string     `json:"id"`
	UserID              string     `json:"user_id"`
	FarmName            string     `json:"farm_name"`
	Location            *string    `json:"location,omitempty"`
	SizeHectares        *float64   `json:"size_hectares,omitempty"`
	OwnershipType       *string    `json:"ownership_type,omitempty"`
	PrimaryCrop         *string    `json:"primary_crop,omitempty"`
	SecondaryCrops      []string   `json:"secondary_crops,omitempty"`
	IrrigationType      *string    `json:"irrigation_type,omitempty"`
	AnnualYieldEstimate *int64     `json:"annual_yield_estimate,omitempty"`
	LastHarvestDate     *time.Time `json:"last_harvest_date,omitempty"`
	IsVerified          bool       `json:"is_verified"`
	VerifiedAt          *time.Time `json:"verified_at,omitempty"`
	VerifiedBy          *string    `json:"verified_by,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// FarmRecordSummaryResponse represents aggregated farm record statistics
type FarmRecordSummaryResponse struct {
	UserID           string           `json:"user_id"`
	StartDate        time.Time        `json:"start_date"`
	EndDate          time.Time        `json:"end_date"`
	TotalRecords     int64            `json:"total_records"`
	VerifiedRecords  int64            `json:"verified_records"`
	TotalSalesAmount int64            `json:"total_sales_amount"`
	TotalExpenses    int64            `json:"total_expenses"`
	TotalInputCosts  int64            `json:"total_input_costs"`
	CropTypes        []string         `json:"crop_types"`
	RecordsByType    map[string]int64 `json:"records_by_type"`
	MonthlySales     map[string]int64 `json:"monthly_sales"`
}
