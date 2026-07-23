package transactionextracted

import "time"

// CreateTransactionExtractedRequest represents the request to create a new extracted transaction
type CreateTransactionExtractedRequest struct {
	UserID          string    `json:"user_id" validate:"required"`
	DocumentID      string    `json:"document_id" validate:"required"`
	TransactionDate time.Time `json:"transaction_date" validate:"required"`
	TransactionType string    `json:"transaction_type" validate:"required"`
	Amount          int64     `json:"amount" validate:"required,gt=0"`
	Counterparty    *string   `json:"counterparty,omitempty"`
	Category        *string   `json:"category,omitempty"`
	Source          string    `json:"source" validate:"required"`
	Description     *string   `json:"description,omitempty"`
	BalanceAfter    *int64    `json:"balance_after,omitempty"`
}

// TransactionExtractedResponse represents the response containing extracted transaction information
type TransactionExtractedResponse struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	DocumentID      string    `json:"document_id"`
	TransactionDate time.Time `json:"transaction_date"`
	TransactionType string    `json:"transaction_type"`
	Amount          int64     `json:"amount"`
	Counterparty    *string   `json:"counterparty,omitempty"`
	Category        *string   `json:"category,omitempty"`
	Source          string    `json:"source"`
	Description     *string   `json:"description,omitempty"`
	BalanceAfter    *int64    `json:"balance_after,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TransactionSummaryResponse represents summary information about transactions
type TransactionSummaryResponse struct {
	TotalCount          int   `json:"total_count"`
	TotalIncome         int64 `json:"total_income"`
	TotalExpenses       int64 `json:"total_expenses"`
	NetCashflow         int64 `json:"net_cashflow"`
	MonthsWithActivity  int   `json:"months_with_activity"`
	UniqueIncomeSources int   `json:"unique_income_sources"`
	AvgMonthlyIncome    int64 `json:"avg_monthly_income"`
	AvgMonthlyExpenses  int64 `json:"avg_monthly_expenses"`
}

// TransactionFilters represents filters for listing extracted transactions
type TransactionFilters struct {
	UserID     string    `json:"user_id,omitempty"`
	DocumentID string    `json:"document_id,omitempty"`
	Category   string    `json:"category,omitempty"`
	Source     string    `json:"source,omitempty"`
	TxType     string    `json:"tx_type,omitempty"`
	StartDate  time.Time `json:"start_date,omitempty"`
	EndDate    time.Time `json:"end_date,omitempty"`
}
