package loan

import "time"

// CreateLoanRequest represents the request to create a new loan
type CreateLoanRequest struct {
	UserID            string  `json:"user_id" validate:"required"`
	AccountID         string  `json:"account_id" validate:"required"`
	ProductID         *string `json:"product_id,omitempty"`
	PrincipalAmount   int64   `json:"principal_amount" validate:"required,gt=0"`
	PrincipalAsset    string  `json:"principal_asset" validate:"required"`
	InterestRateBps   int32   `json:"interest_rate_bps" validate:"required,gt=0"`
	DurationDays      int     `json:"duration_days" validate:"required,gt=0"`
	RepaymentSchedule string  `json:"repayment_schedule" validate:"required"`
}

// UpdateLoanRequest represents the request to update loan information
type UpdateLoanRequest struct {
	VaultTxHash         *string `json:"vault_tx_hash,omitempty"`
	VaultTxStatus       *string `json:"vault_tx_status,omitempty"`
	VaultRepayTxHash    *string `json:"vault_repay_tx_hash,omitempty"`
	RampProvider        *string `json:"ramp_provider,omitempty"`
	RampRequestID       *string `json:"ramp_request_id,omitempty"`
	RampFiatAmount      *int64  `json:"ramp_fiat_amount,omitempty"`
	RampFiatCurr        *string `json:"ramp_fiat_currency,omitempty"`
	MomoProvider        *string `json:"momo_provider,omitempty"`
	MomoTxID            *string `json:"momo_transaction_id,omitempty"`
	MomoStatus          *string `json:"momo_status,omitempty"`
	OriginationFee      *int64  `json:"origination_fee,omitempty"`
	OriginationFeeBps   *int32  `json:"origination_fee_bps,omitempty"`
	TotalAmount         *int64  `json:"total_amount,omitempty"`
	SettlementMethod    *string `json:"settlement_method,omitempty"`
	DisbursementStatus  *string `json:"disbursement_status,omitempty"`
	RampSequenceID      *string `json:"ramp_sequence_id,omitempty"`
	DisbursementRateBps *int64  `json:"disbursement_rate_bps,omitempty"`
	DisbursementAmtKES  *int64  `json:"disbursement_amount_kes,omitempty"`
	RepaymentAmtKES     *int64  `json:"repayment_amount_kes,omitempty"`
	ConversionSpreadBps *int32  `json:"conversion_spread_bps,omitempty"`
	BorrowIndex         *int64  `json:"borrow_index,omitempty"`
	RampFeeUSD          *int64  `json:"ramp_fee_usd,omitempty"`
	RampFeeLocal        *int64  `json:"ramp_fee_local,omitempty"`

	// MoneyGram cash-pickup off-ramp fields (migration 000008). Identifier
	// fields (RampInteractiveURL, RampChildAccountIndex) populate at
	// InitiateOffRamp; the rest (RampExternalRef, RampMoreInfoURL) populate
	// from the poller once MG transitions to pending_user_transfer_complete.
	RampInteractiveURL    *string  `json:"ramp_interactive_url,omitempty"`
	RampExternalRef       *string  `json:"ramp_external_ref,omitempty"`
	RampMoreInfoURL       *string  `json:"ramp_more_info_url,omitempty"`
	RampChildAccountIndex *int32   `json:"ramp_child_account_index,omitempty"`
	RampWithdrawMemo      *string  `json:"ramp_withdraw_memo,omitempty"`
	RampWithdrawMemoType  *string  `json:"ramp_withdraw_memo_type,omitempty"`
	EntryRateUsed         *float64 `json:"entry_rate_used,omitempty"`
	EntryRateSource       *string  `json:"entry_rate_source,omitempty"`
	EntryBufferPct        *float64 `json:"entry_buffer_pct,omitempty"`
	RequestedLocalAmount  *float64 `json:"requested_local_amount,omitempty"`
}

// ApproveLoanRequest represents the request to approve a loan
type ApproveLoanRequest struct {
	ApprovedBy string `json:"approved_by" validate:"required"`
}

// DisburseLoanRequest represents the request to disburse a loan
type DisburseLoanRequest struct {
	VaultTxHash        *string `json:"vault_tx_hash,omitempty"`
	RampProvider       *string `json:"ramp_provider,omitempty"`
	RampRequestID      *string `json:"ramp_request_id,omitempty"`
	RampFiatAmount     *int64  `json:"ramp_fiat_amount,omitempty"`
	RampFiatCurr       *string `json:"ramp_fiat_currency,omitempty"`
	MomoProvider       *string `json:"momo_provider,omitempty"`
	MomoTxID           *string `json:"momo_transaction_id,omitempty"`
	SettlementMethod   *string `json:"settlement_method,omitempty"`
	DisbursementStatus *string `json:"disbursement_status,omitempty"`
	RampSequenceID     *string `json:"ramp_sequence_id,omitempty"`
}

// LoanResponse represents the response containing loan information
type LoanResponse struct {
	ID                  string     `json:"id"`
	LoanNumber          *string    `json:"loan_number,omitempty"`
	UserID              string     `json:"user_id"`
	AccountID           string     `json:"account_id"`
	ProductID           *string    `json:"product_id,omitempty"`
	PrincipalAmount     int64      `json:"principal_amount"`
	PrincipalAsset      string     `json:"principal_asset"`
	InterestRateBps     int32      `json:"interest_rate_bps"`
	InterestAmount      *int64     `json:"interest_amount,omitempty"`
	OriginationFee      *int64     `json:"origination_fee,omitempty"`
	OriginationFeeBps   *int32     `json:"origination_fee_bps,omitempty"`
	TotalAmount         *int64     `json:"total_amount,omitempty"`
	DurationDays        int        `json:"duration_days"`
	RepaymentSched      string     `json:"repayment_schedule"`
	DueDate             *time.Time `json:"due_date,omitempty"`
	Status              string     `json:"status"`
	ApprovedAt          *time.Time `json:"approved_at,omitempty"`
	ApprovedBy          *string    `json:"approved_by,omitempty"`
	DisbursedAt         *time.Time `json:"disbursed_at,omitempty"`
	RepaidAt            *time.Time `json:"repaid_at,omitempty"`
	DefaultedAt         *time.Time `json:"defaulted_at,omitempty"`
	VaultTxHash         *string    `json:"vault_tx_hash,omitempty"`
	VaultTxStatus       *string    `json:"vault_tx_status,omitempty"`
	VaultRepayTxHash    *string    `json:"vault_repay_tx_hash,omitempty"`
	RampProvider        *string    `json:"ramp_provider,omitempty"`
	RampRequestID       *string    `json:"ramp_request_id,omitempty"`
	RampFiatAmount      *int64     `json:"ramp_fiat_amount,omitempty"`
	RampFiatCurr        *string    `json:"ramp_fiat_currency,omitempty"`
	MomoProvider        *string    `json:"momo_provider,omitempty"`
	MomoTxID            *string    `json:"momo_transaction_id,omitempty"`
	MomoStatus          *string    `json:"momo_status,omitempty"`
	SettlementMethod    *string    `json:"settlement_method,omitempty"`
	DisbursementStatus  *string    `json:"disbursement_status,omitempty"`
	RampSequenceID      *string    `json:"ramp_sequence_id,omitempty"`
	DisbursementRateBps *int64     `json:"disbursement_rate_bps,omitempty"`
	DisbursementAmtKES  *int64     `json:"disbursement_amount_kes,omitempty"`
	RepaymentAmtKES     *int64     `json:"repayment_amount_kes,omitempty"`
	ConversionSpreadBps *int32     `json:"conversion_spread_bps,omitempty"`
	BorrowIndex         *int64     `json:"borrow_index,omitempty"`
	RampFeeUSD          *int64     `json:"ramp_fee_usd,omitempty"`
	RampFeeLocal        *int64     `json:"ramp_fee_local,omitempty"`

	// MoneyGram cash-pickup off-ramp fields (migration 000008). Empty for YC loans.
	RampInteractiveURL    *string  `json:"ramp_interactive_url,omitempty"`
	RampExternalRef       *string  `json:"ramp_external_ref,omitempty"`
	RampMoreInfoURL       *string  `json:"ramp_more_info_url,omitempty"`
	RampChildAccountIndex *int32   `json:"ramp_child_account_index,omitempty"`
	RampWithdrawMemo      *string  `json:"ramp_withdraw_memo,omitempty"`
	RampWithdrawMemoType  *string  `json:"ramp_withdraw_memo_type,omitempty"`
	EntryRateUsed         *float64 `json:"entry_rate_used,omitempty"`
	EntryRateSource       *string  `json:"entry_rate_source,omitempty"`
	EntryBufferPct        *float64 `json:"entry_buffer_pct,omitempty"`
	RequestedLocalAmount  *float64 `json:"requested_local_amount,omitempty"`

	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// LoanFilters represents filters for listing loans
type LoanFilters struct {
	UserID string `json:"user_id,omitempty"`
	Status string `json:"status,omitempty"`
}
