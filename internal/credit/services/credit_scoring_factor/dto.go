package creditscoringfactor

import "time"

// CreateCreditScoringFactorRequest represents the request to create a new credit scoring factor
type CreateCreditScoringFactorRequest struct {
	FactorName        string  `json:"factor_name" validate:"required"`
	FactorDescription *string `json:"factor_description,omitempty"`
	WeightBps         int32   `json:"weight_bps" validate:"required,gt=0"`
	IsActive          bool    `json:"is_active"`
	CalculationMethod *string `json:"calculation_method,omitempty"`
	MinDataRequired   int     `json:"min_data_required"`
	CreatedBy         *string `json:"created_by,omitempty"`
}

// UpdateCreditScoringFactorRequest represents the request to update credit scoring factor information
type UpdateCreditScoringFactorRequest struct {
	FactorName        *string `json:"factor_name,omitempty"`
	FactorDescription *string `json:"factor_description,omitempty"`
	WeightBps         *int32  `json:"weight_bps,omitempty"`
	IsActive          *bool   `json:"is_active,omitempty"`
	CalculationMethod *string `json:"calculation_method,omitempty"`
	MinDataRequired   *int    `json:"min_data_required,omitempty"`
	UpdatedBy         *string `json:"updated_by,omitempty"`
}

// CreditScoringFactorResponse represents the response containing credit scoring factor information
type CreditScoringFactorResponse struct {
	ID                string    `json:"id"`
	FactorName        string    `json:"factor_name"`
	FactorDescription *string   `json:"factor_description,omitempty"`
	WeightBps         int32     `json:"weight_bps"`
	IsActive          bool      `json:"is_active"`
	CalculationMethod *string   `json:"calculation_method,omitempty"`
	MinDataRequired   int       `json:"min_data_required"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	CreatedBy         *string   `json:"created_by,omitempty"`
	UpdatedBy         *string   `json:"updated_by,omitempty"`
}
