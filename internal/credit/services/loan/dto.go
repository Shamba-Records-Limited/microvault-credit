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
	OriginationFeeBps int32   `json:"origination_fee_bps,omitempty"` // Zero ⇒ no fee.
	DurationDays      int     `json:"duration_days" validate:"required,gt=0"`
	RepaymentSchedule string  `json:"repayment_schedule" validate:"required"`
}

// UpdateLoanRequest represents the request to update loan information
type UpdateLoanRequest struct {
	VaultTxHash           *string    `json:"vault_tx_hash,omitempty"`
	VaultTxStatus         *string    `json:"vault_tx_status,omitempty"`
	VaultRepayTxHash      *string    `json:"vault_repay_tx_hash,omitempty"`
	VaultRepayStatus      *string    `json:"vault_repay_status,omitempty"`
	RampProvider          *string    `json:"ramp_provider,omitempty"`
	RampRequestID         *string    `json:"ramp_request_id,omitempty"`
	RampFiatAmount        *int64     `json:"ramp_fiat_amount,omitempty"`
	RampFiatCurr          *string    `json:"ramp_fiat_currency,omitempty"`
	MomoProvider          *string    `json:"momo_provider,omitempty"`
	MomoTxID              *string    `json:"momo_transaction_id,omitempty"`
	MomoStatus            *string    `json:"momo_status,omitempty"`
	OriginationFee        *int64     `json:"origination_fee,omitempty"`
	OriginationFeeBps     *int32     `json:"origination_fee_bps,omitempty"`
	TotalAmount           *int64     `json:"total_amount,omitempty"`
	SettlementMethod      *string    `json:"settlement_method,omitempty"`
	DisbursementStatus    *string    `json:"disbursement_status,omitempty"`
	RampSequenceID        *string    `json:"ramp_sequence_id,omitempty"`
	DisbursementRateBps   *int64     `json:"disbursement_rate_bps,omitempty"`
	DeliveredAmtKES       *int64     `json:"delivered_amount_kes,omitempty"`
	QuotedRepaymentAmtKES *int64     `json:"quoted_repayment_amount_kes,omitempty"`
	QuotedAt              *time.Time `json:"quoted_at,omitempty"`
	ConversionSpreadBps   *int32     `json:"conversion_spread_bps,omitempty"`
	BorrowIndex           *int64     `json:"borrow_index,omitempty"`
	ServiceFeeUSD         *int64     `json:"service_fee_usd,omitempty"`
	ServiceFeeLocal       *int64     `json:"service_fee_local,omitempty"`
	PartnerFeeUSD         *int64     `json:"partner_fee_usd,omitempty"`
	PartnerFeeLocal       *int64     `json:"partner_fee_local,omitempty"`

	// MoneyGram cash-pickup fields.
	RampInteractiveURL    *string  `json:"ramp_interactive_url,omitempty"`
	RampShortCode         *string  `json:"ramp_short_code,omitempty"`
	RampExternalRef       *string  `json:"ramp_external_ref,omitempty"`
	RampMoreInfoURL       *string  `json:"ramp_more_info_url,omitempty"`
	RampChildAccountIndex *int64   `json:"ramp_child_account_index,omitempty"`
	RampStellarTxHash     *string  `json:"ramp_stellar_tx_hash,omitempty"`
	EntryRateUsed         *float64 `json:"entry_rate_used,omitempty"`
	EntryRateSource       *string  `json:"entry_rate_source,omitempty"`
	EntryBufferPct        *float64 `json:"entry_buffer_pct,omitempty"`
	RequestedLocalAmount  *int64   `json:"requested_local_amount,omitempty"`
	RampWithdrawMemo      *string  `json:"ramp_withdraw_memo,omitempty"`
	RampWithdrawMemoType  *string  `json:"ramp_withdraw_memo_type,omitempty"`

	RampRefundTxHash    *string    `json:"ramp_refund_tx_hash,omitempty"`
	RampRefundAmount    *int64     `json:"ramp_refund_amount,omitempty"`
	RampRefundShortfall *int64     `json:"ramp_refund_shortfall,omitempty"`
	RampRefundedAt      *time.Time `json:"ramp_refunded_at,omitempty"`
}

// ApproveLoanRequest represents the request to approve a loan
type ApproveLoanRequest struct {
	ApprovedBy string `json:"approved_by" validate:"required"`
}

// DisburseLoanRequest represents the request to disburse a loan
type DisburseLoanRequest struct {
	VaultTxHash        *string `json:"vault_tx_hash,omitempty"`
	VaultTxStatus      *string `json:"vault_tx_status,omitempty"`
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
	ID                    string     `json:"id"`
	LoanReference         *string    `json:"loan_reference,omitempty"`
	UserID                string     `json:"user_id"`
	AccountID             string     `json:"account_id"`
	ProductID             *string    `json:"product_id,omitempty"`
	PrincipalAmount       int64      `json:"principal_amount"`
	PrincipalAsset        string     `json:"principal_asset"`
	InterestRateBps       int32      `json:"interest_rate_bps"`
	InterestAmount        *int64     `json:"interest_amount,omitempty"`
	OriginationFee        *int64     `json:"origination_fee,omitempty"`
	OriginationFeeBps     *int32     `json:"origination_fee_bps,omitempty"`
	TotalAmount           *int64     `json:"total_amount,omitempty"`
	DurationDays          int        `json:"duration_days"`
	RepaymentSched        string     `json:"repayment_schedule"`
	DueDate               *time.Time `json:"due_date,omitempty"`
	Status                string     `json:"status"`
	ApprovedAt            *time.Time `json:"approved_at,omitempty"`
	ApprovedBy            *string    `json:"approved_by,omitempty"`
	DisbursedAt           *time.Time `json:"disbursed_at,omitempty"`
	RepaidAt              *time.Time `json:"repaid_at,omitempty"`
	DefaultedAt           *time.Time `json:"defaulted_at,omitempty"`
	VaultTxHash           *string    `json:"vault_tx_hash,omitempty"`
	VaultTxStatus         *string    `json:"vault_tx_status,omitempty"`
	VaultRepayTxHash      *string    `json:"vault_repay_tx_hash,omitempty"`
	VaultRepayStatus      *string    `json:"vault_repay_status,omitempty"`
	RampProvider          *string    `json:"ramp_provider,omitempty"`
	RampRequestID         *string    `json:"ramp_request_id,omitempty"`
	RampFiatAmount        *int64     `json:"ramp_fiat_amount,omitempty"`
	RampFiatCurr          *string    `json:"ramp_fiat_currency,omitempty"`
	MomoProvider          *string    `json:"momo_provider,omitempty"`
	MomoTxID              *string    `json:"momo_transaction_id,omitempty"`
	MomoStatus            *string    `json:"momo_status,omitempty"`
	SettlementMethod      *string    `json:"settlement_method,omitempty"`
	DisbursementStatus    *string    `json:"disbursement_status,omitempty"`
	RampSequenceID        *string    `json:"ramp_sequence_id,omitempty"`
	DisbursementRateBps   *int64     `json:"disbursement_rate_bps,omitempty"`
	DeliveredAmtKES       *int64     `json:"delivered_amount_kes,omitempty"`
	QuotedRepaymentAmtKES *int64     `json:"quoted_repayment_amount_kes,omitempty"`
	QuotedAt              *time.Time `json:"quoted_at,omitempty"`
	ConversionSpreadBps   *int32     `json:"conversion_spread_bps,omitempty"`
	BorrowIndex           *int64     `json:"borrow_index,omitempty"`
	ServiceFeeUSD         *int64     `json:"service_fee_usd,omitempty"`
	ServiceFeeLocal       *int64     `json:"service_fee_local,omitempty"`
	PartnerFeeUSD         *int64     `json:"partner_fee_usd,omitempty"`
	PartnerFeeLocal       *int64     `json:"partner_fee_local,omitempty"`

	RampInteractiveURL    *string  `json:"ramp_interactive_url,omitempty"`
	RampShortCode         *string  `json:"ramp_short_code,omitempty"`
	RampExternalRef       *string  `json:"ramp_external_ref,omitempty"`
	RampMoreInfoURL       *string  `json:"ramp_more_info_url,omitempty"`
	RampChildAccountIndex *int64   `json:"ramp_child_account_index,omitempty"`
	RampStellarTxHash     *string  `json:"ramp_stellar_tx_hash,omitempty"`
	EntryRateUsed         *float64 `json:"entry_rate_used,omitempty"`
	EntryRateSource       *string  `json:"entry_rate_source,omitempty"`
	EntryBufferPct        *float64 `json:"entry_buffer_pct,omitempty"`
	RequestedLocalAmount  *int64   `json:"requested_local_amount,omitempty"`
	RampWithdrawMemo      *string  `json:"ramp_withdraw_memo,omitempty"`
	RampWithdrawMemoType  *string  `json:"ramp_withdraw_memo_type,omitempty"`

	RampRefundTxHash    *string    `json:"ramp_refund_tx_hash,omitempty"`
	RampRefundAmount    *int64     `json:"ramp_refund_amount,omitempty"`
	RampRefundShortfall *int64     `json:"ramp_refund_shortfall,omitempty"`
	RampRefundedAt      *time.Time `json:"ramp_refunded_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LoanFilters represents filters for listing loans
type LoanFilters struct {
	UserID string `json:"user_id,omitempty"`
	Status string `json:"status,omitempty"`
}

// changedFields returns only the columns this request actually sets, so a
// partial update writes a handful of columns instead of rewriting the whole
// row (including the ~1KB SEP-24 URLs) and every index on it.
//
// This is the single place the request-to-column mapping lives; the repository
// validates the keys against its own allow-list.
func (r UpdateLoanRequest) changedFields() map[string]any {
	f := make(map[string]any, 39)
	if r.VaultTxHash != nil {
		f["vault_tx_hash"] = r.VaultTxHash
	}
	if r.VaultTxStatus != nil {
		f["vault_tx_status"] = r.VaultTxStatus
	}
	if r.VaultRepayTxHash != nil {
		f["vault_repay_tx_hash"] = r.VaultRepayTxHash
	}
	if r.VaultRepayStatus != nil {
		f["vault_repay_status"] = r.VaultRepayStatus
	}
	if r.RampProvider != nil {
		f["ramp_provider"] = r.RampProvider
	}
	if r.RampRequestID != nil {
		f["ramp_request_id"] = r.RampRequestID
	}
	if r.RampFiatAmount != nil {
		f["ramp_fiat_amount"] = r.RampFiatAmount
	}
	if r.RampFiatCurr != nil {
		f["ramp_fiat_currency"] = r.RampFiatCurr
	}
	if r.MomoProvider != nil {
		f["momo_provider"] = r.MomoProvider
	}
	if r.MomoTxID != nil {
		f["momo_transaction_id"] = r.MomoTxID
	}
	if r.MomoStatus != nil {
		f["momo_status"] = r.MomoStatus
	}
	if r.OriginationFee != nil {
		f["origination_fee"] = r.OriginationFee
	}
	if r.OriginationFeeBps != nil {
		f["origination_fee_bps"] = r.OriginationFeeBps
	}
	if r.TotalAmount != nil {
		f["total_amount"] = r.TotalAmount
	}
	if r.SettlementMethod != nil {
		f["settlement_method"] = r.SettlementMethod
	}
	if r.DisbursementStatus != nil {
		f["disbursement_status"] = r.DisbursementStatus
	}
	if r.RampSequenceID != nil {
		f["ramp_sequence_id"] = r.RampSequenceID
	}
	if r.DisbursementRateBps != nil {
		f["disbursement_rate_bps"] = r.DisbursementRateBps
	}
	if r.DeliveredAmtKES != nil {
		f["delivered_amount_kes"] = r.DeliveredAmtKES
	}
	if r.QuotedRepaymentAmtKES != nil {
		f["quoted_repayment_amount_kes"] = r.QuotedRepaymentAmtKES
	}
	if r.QuotedAt != nil {
		f["quoted_at"] = r.QuotedAt
	}
	if r.ConversionSpreadBps != nil {
		f["conversion_spread_bps"] = r.ConversionSpreadBps
	}
	if r.BorrowIndex != nil {
		f["borrow_index"] = r.BorrowIndex
	}
	if r.ServiceFeeUSD != nil {
		f["service_fee_usd"] = r.ServiceFeeUSD
	}
	if r.ServiceFeeLocal != nil {
		f["service_fee_local"] = r.ServiceFeeLocal
	}
	if r.PartnerFeeUSD != nil {
		f["partner_fee_usd"] = r.PartnerFeeUSD
	}
	if r.PartnerFeeLocal != nil {
		f["partner_fee_local"] = r.PartnerFeeLocal
	}
	if r.RampInteractiveURL != nil {
		f["ramp_interactive_url"] = r.RampInteractiveURL
	}
	if r.RampShortCode != nil {
		f["ramp_short_code"] = r.RampShortCode
	}
	if r.RampExternalRef != nil {
		f["ramp_external_ref"] = r.RampExternalRef
	}
	if r.RampMoreInfoURL != nil {
		f["ramp_more_info_url"] = r.RampMoreInfoURL
	}
	if r.RampStellarTxHash != nil {
		f["ramp_stellar_tx_hash"] = r.RampStellarTxHash
	}
	if r.RampChildAccountIndex != nil {
		f["ramp_child_account_index"] = r.RampChildAccountIndex
	}
	if r.EntryRateUsed != nil {
		f["entry_rate_used"] = r.EntryRateUsed
	}
	if r.EntryRateSource != nil {
		f["entry_rate_source"] = r.EntryRateSource
	}
	if r.EntryBufferPct != nil {
		f["entry_buffer_pct"] = r.EntryBufferPct
	}
	if r.RequestedLocalAmount != nil {
		f["requested_local_amount"] = r.RequestedLocalAmount
	}
	if r.RampWithdrawMemo != nil {
		f["ramp_withdraw_memo"] = r.RampWithdrawMemo
	}
	if r.RampWithdrawMemoType != nil {
		f["ramp_withdraw_memo_type"] = r.RampWithdrawMemoType
	}
	if r.RampRefundTxHash != nil {
		f["ramp_refund_tx_hash"] = r.RampRefundTxHash
	}
	if r.RampRefundAmount != nil {
		f["ramp_refund_amount"] = r.RampRefundAmount
	}
	if r.RampRefundShortfall != nil {
		f["ramp_refund_shortfall"] = r.RampRefundShortfall
	}
	if r.RampRefundedAt != nil {
		f["ramp_refunded_at"] = r.RampRefundedAt
	}
	return f
}
