package repayment

import "time"

// CreateRepaymentRequest represents the request to create a new repayment
type CreateRepaymentRequest struct {
	LoanID            string    `json:"loan_id" validate:"required"`
	UserID            string    `json:"user_id" validate:"required"`
	InstallmentNumber int       `json:"installment_number" validate:"required,gt=0"`
	DueDate           time.Time `json:"due_date" validate:"required"`
	AmountDue         int64     `json:"amount_due" validate:"required,gt=0"`
}

// RecordPaymentRequest represents the request to record a payment
type RecordPaymentRequest struct {
	AmountPaid    int64   `json:"amount_paid" validate:"required,gt=0"`
	PaymentMethod *string `json:"payment_method,omitempty"`
	TransactionID *string `json:"transaction_id,omitempty"`
}

// UpdateRepaymentRequest represents the request to update repayment information
type UpdateRepaymentRequest struct {
	AmountDue *int64     `json:"amount_due,omitempty"`
	DueDate   *time.Time `json:"due_date,omitempty"`
	LateFee   *int64     `json:"late_fee,omitempty"`
}

// RepaymentResponse represents the response containing repayment information
type RepaymentResponse struct {
	ID                string     `json:"id"`
	LoanID            string     `json:"loan_id"`
	UserID            string     `json:"user_id"`
	InstallmentNumber int        `json:"installment_number"`
	DueDate           time.Time  `json:"due_date"`
	AmountDue         int64      `json:"amount_due"`
	AmountPaid        int64      `json:"amount_paid"`
	PaidAt            *time.Time `json:"paid_at,omitempty"`
	PaymentMethod     *string    `json:"payment_method,omitempty"`
	Status            string     `json:"status"`
	LateFee           int64      `json:"late_fee"`
	TransactionID     *string    `json:"transaction_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// RepaymentFilters represents filters for listing repayments
type RepaymentFilters struct {
	LoanID string `json:"loan_id,omitempty"`
	UserID string `json:"user_id,omitempty"`
	Status string `json:"status,omitempty"`
}
