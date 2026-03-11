package loanproduct

import "time"

// CreateLoanProductRequest represents the request to create a new loan product.
// MinAmount and MaxAmount are in fiat cents denominated in Currency.
type CreateLoanProductRequest struct {
	Name                      string   `json:"name" validate:"required"`
	Description               *string  `json:"description,omitempty"`
	InterestRateBps           int32    `json:"interest_rate_bps" validate:"required,gt=0"`
	InterestType              string   `json:"interest_type" validate:"required"`
	OriginationFeeBps         *int32   `json:"origination_fee_bps,omitempty"`
	MinAmount                 int64    `json:"min_amount" validate:"required,gt=0"`
	MaxAmount                 int64    `json:"max_amount" validate:"required,gt=0"`
	Currency                  string   `json:"currency" validate:"required"`
	MinDurationDays           int      `json:"min_duration_days" validate:"required,gt=0"`
	MaxDurationDays           int      `json:"max_duration_days" validate:"required,gt=0"`
	AllowedRepaymentSchedules []string `json:"allowed_repayment_schedules" validate:"required"`
	MaxCreditMultiplierBps    int32    `json:"max_credit_multiplier_bps" validate:"required,gt=0"`
	RequiresCollateral        bool     `json:"requires_collateral"`
	CollateralBps             *int32   `json:"collateral_bps,omitempty"`
	PriorityOrder             int      `json:"priority_order"`
	IsActive                  bool     `json:"is_active"`
}

// UpdateLoanProductRequest represents the request to update loan product information.
type UpdateLoanProductRequest struct {
	Name                      *string  `json:"name,omitempty"`
	Description               *string  `json:"description,omitempty"`
	InterestRateBps           *int32   `json:"interest_rate_bps,omitempty"`
	InterestType              *string  `json:"interest_type,omitempty"`
	OriginationFeeBps         *int32   `json:"origination_fee_bps,omitempty"`
	MinAmount                 *int64   `json:"min_amount,omitempty"`
	MaxAmount                 *int64   `json:"max_amount,omitempty"`
	Currency                  *string  `json:"currency,omitempty"`
	MinDurationDays           *int     `json:"min_duration_days,omitempty"`
	MaxDurationDays           *int     `json:"max_duration_days,omitempty"`
	AllowedRepaymentSchedules []string `json:"allowed_repayment_schedules,omitempty"`
	MaxCreditMultiplierBps    *int32   `json:"max_credit_multiplier_bps,omitempty"`
	RequiresCollateral        *bool    `json:"requires_collateral,omitempty"`
	CollateralBps             *int32   `json:"collateral_bps,omitempty"`
	PriorityOrder             *int     `json:"priority_order,omitempty"`
	IsActive                  *bool    `json:"is_active,omitempty"`
}

// LoanProductResponse represents the response containing loan product information.
type LoanProductResponse struct {
	ID                        string    `json:"id"`
	Name                      string    `json:"name"`
	Description               *string   `json:"description,omitempty"`
	InterestRateBps           int32     `json:"interest_rate_bps"`
	InterestType              string    `json:"interest_type"`
	OriginationFeeBps         *int32    `json:"origination_fee_bps,omitempty"`
	MinAmount                 int64     `json:"min_amount"`
	MaxAmount                 int64     `json:"max_amount"`
	Currency                  string    `json:"currency"`
	MinDurationDays           int       `json:"min_duration_days"`
	MaxDurationDays           int       `json:"max_duration_days"`
	AllowedRepaymentSchedules []string  `json:"allowed_repayment_schedules"`
	MaxCreditMultiplierBps    int32     `json:"max_credit_multiplier_bps"`
	RequiresCollateral        bool      `json:"requires_collateral"`
	CollateralBps             *int32    `json:"collateral_bps,omitempty"`
	PriorityOrder             int       `json:"priority_order"`
	IsActive                  bool      `json:"is_active"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}
