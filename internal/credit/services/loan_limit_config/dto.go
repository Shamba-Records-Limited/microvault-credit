package loanlimitconfig

import "time"

// CreateLoanLimitConfigRequest represents the request to create a new loan limit config
type CreateLoanLimitConfigRequest struct {
	RiskTier            string  `json:"risk_tier" validate:"required"`
	MinLoanAmount       int64   `json:"min_loan_amount" validate:"required,gt=0"`
	MaxLoanAmount       int64   `json:"max_loan_amount" validate:"required,gt=0"`
	IncomeMultiplierBps int32   `json:"income_multiplier_bps" validate:"required,gt=0"`
	MaxConcurrentLoans  int     `json:"max_concurrent_loans" validate:"required,gt=0"`
	MaxLoanDurationDays int     `json:"max_loan_duration_days" validate:"required,gt=0"`
	InterestRateBps     int32   `json:"interest_rate_bps" validate:"required,gt=0"`
	LateFeeBps          *int32  `json:"late_fee_bps,omitempty"`
	DefaultPenaltyBps   *int32  `json:"default_penalty_bps,omitempty"`
	IsActive            bool    `json:"is_active"`
	CreatedBy           *string `json:"created_by,omitempty"`
}

// UpdateLoanLimitConfigRequest represents the request to update loan limit config information
type UpdateLoanLimitConfigRequest struct {
	RiskTier            *string `json:"risk_tier,omitempty"`
	MinLoanAmount       *int64  `json:"min_loan_amount,omitempty"`
	MaxLoanAmount       *int64  `json:"max_loan_amount,omitempty"`
	IncomeMultiplierBps *int32  `json:"income_multiplier_bps,omitempty"`
	MaxConcurrentLoans  *int    `json:"max_concurrent_loans,omitempty"`
	MaxLoanDurationDays *int    `json:"max_loan_duration_days,omitempty"`
	InterestRateBps     *int32  `json:"interest_rate_bps,omitempty"`
	LateFeeBps          *int32  `json:"late_fee_bps,omitempty"`
	DefaultPenaltyBps   *int32  `json:"default_penalty_bps,omitempty"`
	IsActive            *bool   `json:"is_active,omitempty"`
	UpdatedBy           *string `json:"updated_by,omitempty"`
}

// LoanLimitConfigResponse represents the response containing loan limit config information
type LoanLimitConfigResponse struct {
	ID                  string    `json:"id"`
	RiskTier            string    `json:"risk_tier"`
	MinLoanAmount       int64     `json:"min_loan_amount"`
	MaxLoanAmount       int64     `json:"max_loan_amount"`
	IncomeMultiplierBps int32     `json:"income_multiplier_bps"`
	MaxConcurrentLoans  int       `json:"max_concurrent_loans"`
	MaxLoanDurationDays int       `json:"max_loan_duration_days"`
	InterestRateBps     int32     `json:"interest_rate_bps"`
	LateFeeBps          *int32    `json:"late_fee_bps,omitempty"`
	DefaultPenaltyBps   *int32    `json:"default_penalty_bps,omitempty"`
	IsActive            bool      `json:"is_active"`
	CreatedAt           time.Time `json:"created_at"`
	CreatedBy           *string   `json:"created_by,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
	UpdatedBy           *string   `json:"updated_by,omitempty"`
}
