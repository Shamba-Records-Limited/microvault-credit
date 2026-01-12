package creditfactor

import "time"

// CreateCreditFactorRequest represents the request to create a new credit factor
type CreateCreditFactorRequest struct {
	UserID            string  `json:"user_id" validate:"required"`
	CreditScoreID     *string `json:"credit_score_id,omitempty"`
	FactorName        string  `json:"factor_name" validate:"required"`
	FactorValueBps    int32   `json:"factor_value_bps" validate:"required"`
	FactorWeightBps   int32   `json:"factor_weight_bps" validate:"required"`
	CalculationMethod *string `json:"calculation_method,omitempty"`
	DataSource        *string `json:"data_source,omitempty"`
}

// UpdateCreditFactorRequest represents the request to update credit factor information
type UpdateCreditFactorRequest struct {
	FactorValueBps    *int32  `json:"factor_value_bps,omitempty"`
	FactorWeightBps   *int32  `json:"factor_weight_bps,omitempty"`
	CalculationMethod *string `json:"calculation_method,omitempty"`
	DataSource        *string `json:"data_source,omitempty"`
}

// CreditFactorResponse represents the response containing credit factor information
type CreditFactorResponse struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	CreditScoreID     *string   `json:"credit_score_id,omitempty"`
	FactorName        string    `json:"factor_name"`
	FactorValueBps    int32     `json:"factor_value_bps"`
	FactorWeightBps   int32     `json:"factor_weight_bps"`
	WeightedScoreBps  int32     `json:"weighted_score_bps"`
	CalculationMethod *string   `json:"calculation_method,omitempty"`
	DataSource        *string   `json:"data_source,omitempty"`
	CalculatedAt      time.Time `json:"calculated_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
