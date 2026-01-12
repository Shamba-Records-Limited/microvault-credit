package creditscore

import "time"

// CreateCreditScoreRequest represents the request to create a new credit score
type CreateCreditScoreRequest struct {
	UserID               string     `json:"user_id" validate:"required"`
	Score                int        `json:"score" validate:"required,min=0,max=1000"`
	ScoreVersion         string     `json:"score_version" validate:"required"`
	TotalLoans           int        `json:"total_loans"`
	SuccessfulRepayments int        `json:"successful_repayments"`
	Defaults             int        `json:"defaults"`
	CurrentOutstanding   int64      `json:"current_outstanding"`
	DaysOverdue          int        `json:"days_overdue"`
	MaxLoanAmount        *int64     `json:"max_loan_amount,omitempty"`
	MaxConcurrentLoans   int        `json:"max_concurrent_loans"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
}

// UpdateCreditScoreRequest represents the request to update credit score information
type UpdateCreditScoreRequest struct {
	Score                *int       `json:"score,omitempty"`
	ScoreVersion         *string    `json:"score_version,omitempty"`
	TotalLoans           *int       `json:"total_loans,omitempty"`
	SuccessfulRepayments *int       `json:"successful_repayments,omitempty"`
	Defaults             *int       `json:"defaults,omitempty"`
	CurrentOutstanding   *int64     `json:"current_outstanding,omitempty"`
	DaysOverdue          *int       `json:"days_overdue,omitempty"`
	MaxLoanAmount        *int64     `json:"max_loan_amount,omitempty"`
	MaxConcurrentLoans   *int       `json:"max_concurrent_loans,omitempty"`
}

// CreditScoreResponse represents the response containing credit score information
type CreditScoreResponse struct {
	ID                   string     `json:"id"`
	UserID               string     `json:"user_id"`
	Score                int        `json:"score"`
	ScoreVersion         string     `json:"score_version"`
	TotalLoans           int        `json:"total_loans"`
	SuccessfulRepayments int        `json:"successful_repayments"`
	Defaults             int        `json:"defaults"`
	CurrentOutstanding   int64      `json:"current_outstanding"`
	DaysOverdue          int        `json:"days_overdue"`
	MaxLoanAmount        *int64     `json:"max_loan_amount,omitempty"`
	MaxConcurrentLoans   int        `json:"max_concurrent_loans"`
	CalculatedAt         time.Time  `json:"calculated_at"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	IsExpired            bool       `json:"is_expired"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}
