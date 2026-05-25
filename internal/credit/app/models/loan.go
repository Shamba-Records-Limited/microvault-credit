package models

import (
	"fmt"
	"time"

	transactions "github.com/Shamba-Records-Limited/microvault/pkg/models"
	users "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Loan represents a loan record
// Amounts stored in smallest unit, rates in basis points
type Loan struct {
	ID                    string     `json:"id" gorm:"type:uuid;primaryKey"`
	LoanReference         *string    `json:"loan_reference,omitempty" gorm:"column:loan_reference;type:varchar(50);uniqueIndex"`
	UserID                string     `json:"user_id" gorm:"type:uuid;not null;index"`
	AccountID             string     `json:"account_id" gorm:"type:uuid;not null;index"`
	ProductID             *string    `json:"product_id,omitempty" gorm:"type:uuid;index"`
	PrincipalAmount       int64      `json:"principal_amount" gorm:"type:bigint;not null"`
	PrincipalAsset        string     `json:"principal_asset" gorm:"type:varchar(20);not null;index"`
	InterestRateBps       int32      `json:"interest_rate_bps" gorm:"type:int;not null"`
	InterestAmount        *int64     `json:"interest_amount,omitempty" gorm:"type:bigint"`
	OriginationFee        *int64     `json:"origination_fee,omitempty" gorm:"type:bigint"`
	OriginationFeeBps     *int32     `json:"origination_fee_bps,omitempty" gorm:"type:int"`
	TotalAmount           *int64     `json:"total_amount,omitempty" gorm:"type:bigint"`
	DurationDays          int        `json:"duration_days" gorm:"type:int;not null"`
	RepaymentSchedule     string     `json:"repayment_schedule" gorm:"type:varchar(20);not null"`
	DueDate               *time.Time `json:"due_date,omitempty" gorm:"type:timestamp;index"`
	Status                string     `json:"status" gorm:"type:varchar(20);not null;default:'pending';index"`
	ApprovedAt            *time.Time `json:"approved_at,omitempty" gorm:"type:timestamp"`
	ApprovedBy            *string    `json:"approved_by,omitempty" gorm:"type:uuid"`
	DisbursedAt           *time.Time `json:"disbursed_at,omitempty" gorm:"type:timestamp;index"`
	RepaidAt              *time.Time `json:"repaid_at,omitempty" gorm:"type:timestamp"`
	DefaultedAt           *time.Time `json:"defaulted_at,omitempty" gorm:"type:timestamp"`
	VaultTxHash           *string    `json:"vault_tx_hash,omitempty" gorm:"type:varchar(64);index"`
	VaultTxStatus         *string    `json:"vault_tx_status,omitempty" gorm:"type:varchar(20)"`
	VaultRepayTxHash      *string    `json:"vault_repay_tx_hash,omitempty" gorm:"type:varchar(64);index"`
	VaultRepayStatus      *string    `json:"vault_repay_status,omitempty" gorm:"type:varchar(20);index"`
	RampProvider          *string    `json:"ramp_provider,omitempty" gorm:"type:varchar(50)"`
	RampRequestID         *string    `json:"ramp_request_id,omitempty" gorm:"type:varchar(100);index"`
	RampFiatAmount        *int64     `json:"ramp_fiat_amount,omitempty" gorm:"type:bigint"`
	RampFiatCurr          *string    `json:"ramp_fiat_currency,omitempty" gorm:"column:ramp_fiat_currency;type:varchar(10)"`
	MomoProvider          *string    `json:"momo_provider,omitempty" gorm:"type:varchar(50)"`
	MomoTxID              *string    `json:"momo_transaction_id,omitempty" gorm:"column:momo_transaction_id;type:varchar(100);index"`
	MomoStatus            *string    `json:"momo_status,omitempty" gorm:"type:varchar(20)"`
	SettlementMethod      *string    `json:"settlement_method,omitempty" gorm:"type:varchar(20)"`
	DisbursementStatus    *string    `json:"disbursement_status,omitempty" gorm:"type:varchar(30);index"`
	RampSequenceID        *string    `json:"ramp_sequence_id,omitempty" gorm:"type:varchar(200);index"`
	DisbursementRateBps   *int64     `json:"disbursement_rate_bps,omitempty" gorm:"type:bigint"`
	DeliveredAmtKES       *int64     `json:"delivered_amount_kes,omitempty" gorm:"column:delivered_amount_kes;type:bigint"`
	QuotedRepaymentAmtKES *int64     `json:"quoted_repayment_amount_kes,omitempty" gorm:"column:quoted_repayment_amount_kes;type:bigint"`
	QuotedAt              *time.Time `json:"quoted_at,omitempty" gorm:"column:quoted_at;type:timestamp"`
	ConversionSpreadBps   *int32     `json:"conversion_spread_bps,omitempty" gorm:"type:int"`
	BorrowIndex           *int64     `json:"borrow_index,omitempty" gorm:"type:bigint"`
	RampFeeUSD            *int64     `json:"ramp_fee_usd,omitempty" gorm:"type:bigint"`
	RampFeeLocal          *int64     `json:"ramp_fee_local,omitempty" gorm:"type:bigint"`

	// MoneyGram cash-pickup fields (see migration 000008).
	// RampInteractiveURL is the SEP-24 webview URL sent to the user via SMS.
	// RampExternalRef is the cash-pickup reference number returned post-completion.
	// RampMoreInfoURL is MG's support deep-link for the transaction.
	// RampChildAccountIndex is the per-user Stellar derivation index used to
	// re-derive the SEP-10 child memo on poller restart.
	RampInteractiveURL    *string `json:"ramp_interactive_url,omitempty" gorm:"type:text"`
	RampExternalRef       *string `json:"ramp_external_ref,omitempty" gorm:"type:varchar(100);index"`
	RampMoreInfoURL       *string `json:"ramp_more_info_url,omitempty" gorm:"type:text"`
	RampChildAccountIndex *int64  `json:"ramp_child_account_index,omitempty" gorm:"type:bigint"`

	// FX audit fields capture the rate the loan was quoted at, the source
	// label, the entry buffer percentage applied, and the user's originally
	// requested local amount — used by the poller's drift detection.
	EntryRateUsed        *float64 `json:"entry_rate_used,omitempty" gorm:"type:numeric(20,8)"`
	EntryRateSource      *string  `json:"entry_rate_source,omitempty" gorm:"type:varchar(40)"`
	EntryBufferPct       *float64 `json:"entry_buffer_pct,omitempty" gorm:"type:numeric(6,4)"`
	RequestedLocalAmount *int64   `json:"requested_local_amount,omitempty" gorm:"type:bigint"`

	// SEP-24 withdraw memo returned on MG's transaction object; used by the
	// Stellar ingest worker to match refund inbound USDC back to the loan.
	RampWithdrawMemo     *string `json:"ramp_withdraw_memo,omitempty" gorm:"type:varchar(64);index"`
	RampWithdrawMemoType *string `json:"ramp_withdraw_memo_type,omitempty" gorm:"type:varchar(10)"`

	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"autoUpdateTime;not null"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	User       *users.User    `gorm:"foreignKey:UserID"`
	Account    *users.Account `gorm:"foreignKey:AccountID"`
	Product    *LoanProduct   `gorm:"foreignKey:ProductID"`
	Repayments []Repayment    `gorm:"foreignKey:LoanID"`
}

// TableName specifies the table name for Loan model
func (Loan) TableName() string {
	return "loans"
}

// BeforeCreate sets the ID and generates a loan number before creating a new loan.
func (loan *Loan) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	loan.ID = id.String()

	// Generate a human-readable loan reference from the UUIDv7 timestamp +
	// random suffix.
	// Format: LR-unix_ms_hex-4_random_hex e.g. "LR-018F3A2B1C-A7F2"
	if loan.LoanReference == nil {
		ts := time.Now().UnixMilli()
		short := id.String()[24:28] // 4 hex chars from the random portion
		ref := fmt.Sprintf("LR-%X-%s", ts, short)
		loan.LoanReference = &ref
	}
	return nil
}

// LoanProduct represents a loan product with specific terms.
// MinAmount and MaxAmount are denominated in the fiat Currency (stored as cents).
// Rates are in basis points (1 bps = 0.01 %).
type LoanProduct struct {
	ID                        string                      `json:"id" gorm:"type:uuid;primaryKey"`
	Name                      string                      `json:"name" gorm:"type:varchar(100);uniqueIndex;not null"`
	Description               *string                     `json:"description,omitempty" gorm:"type:text"`
	InterestRateBps           int32                       `json:"interest_rate_bps" gorm:"type:int;not null"`
	InterestType              string                      `json:"interest_type" gorm:"type:varchar(20);not null;default:'simple'"`
	OriginationFeeBps         *int32                      `json:"origination_fee_bps,omitempty" gorm:"type:int"`
	MinAmount                 int64                       `json:"min_amount" gorm:"type:bigint;not null"`
	MaxAmount                 int64                       `json:"max_amount" gorm:"type:bigint;not null"`
	Currency                  string                      `json:"currency" gorm:"type:varchar(10);not null;default:'KES'"`
	MinDurationDays           int                         `json:"min_duration_days" gorm:"type:int;not null"`
	MaxDurationDays           int                         `json:"max_duration_days" gorm:"type:int;not null"`
	AllowedRepaymentSchedules datatypes.JSONSlice[string] `json:"allowed_repayment_schedules" gorm:"type:jsonb"`
	MaxCreditMultiplierBps    int32                       `json:"max_credit_multiplier_bps" gorm:"type:int;not null"`
	RequiresCollateral        bool                        `json:"requires_collateral" gorm:"type:boolean;not null;default:false"`
	CollateralBps             *int32                      `json:"collateral_bps,omitempty" gorm:"type:int"`
	PriorityOrder             int                         `json:"priority_order" gorm:"type:int;not null;default:0"`
	IsActive                  bool                        `json:"is_active" gorm:"type:boolean;not null;default:true;index"`
	CreatedAt                 time.Time                   `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt                 time.Time                   `json:"updated_at" gorm:"autoUpdateTime;not null"`
	DeletedAt                 gorm.DeletedAt              `json:"deleted_at" gorm:"index"`
}

// TableName specifies the table name for LoanProduct model
func (LoanProduct) TableName() string {
	return "loan_products"
}

// BeforeCreate sets the ID before creating a new loan product
func (lp *LoanProduct) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	lp.ID = id.String()
	return nil
}

// CreditScore represents a user's credit score
type CreditScore struct {
	ID                   string         `json:"id" gorm:"type:uuid;primaryKey"`
	UserID               string         `json:"user_id" gorm:"type:uuid;not null;uniqueIndex"`
	Score                int            `json:"score" gorm:"type:int;not null;index"`
	ScoreVersion         string         `json:"score_version" gorm:"type:varchar(20);not null"`
	TotalLoans           int            `json:"total_loans" gorm:"type:int;not null;default:0"`
	SuccessfulRepayments int            `json:"successful_repayments" gorm:"type:int;not null;default:0"`
	Defaults             int            `json:"defaults" gorm:"type:int;not null;default:0"`
	CurrentOutstanding   int64          `json:"current_outstanding" gorm:"type:bigint;not null;default:0"`
	DaysOverdue          int            `json:"days_overdue" gorm:"type:int;not null;default:0"`
	MaxLoanAmount        *int64         `json:"max_loan_amount,omitempty" gorm:"type:bigint"`
	MaxConcurrentLoans   int            `json:"max_concurrent_loans" gorm:"type:int;not null;default:1"`
	CalculatedAt         time.Time      `json:"calculated_at" gorm:"type:timestamp;not null"`
	ExpiresAt            *time.Time     `json:"expires_at,omitempty" gorm:"type:timestamp;index"`
	CreatedAt            time.Time      `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt            time.Time      `json:"updated_at" gorm:"autoUpdateTime;not null"`
	DeletedAt            gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	User *users.User `gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for CreditScore model
func (CreditScore) TableName() string {
	return "credit_scores"
}

// BeforeCreate sets the ID before creating a new credit score
func (cs *CreditScore) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	cs.ID = id.String()
	return nil
}

// IsExpired checks if the credit score has expired
func (r *CreditScore) IsExpired() bool {
	if r.ExpiresAt == nil {
		return false
	}
	return r.ExpiresAt.Before(time.Now())
}

// Repayment represents a loan repayment schedule item
type Repayment struct {
	ID                string         `json:"id" gorm:"type:uuid;primaryKey"`
	LoanID            string         `json:"loan_id" gorm:"type:uuid;not null;index"`
	UserID            string         `json:"user_id" gorm:"type:uuid;not null;index"`
	InstallmentNumber int            `json:"installment_number" gorm:"type:int;not null"`
	DueDate           time.Time      `json:"due_date" gorm:"type:timestamp;not null;index"`
	AmountDue         int64          `json:"amount_due" gorm:"type:bigint;not null"`
	AmountPaid        int64          `json:"amount_paid" gorm:"type:bigint;not null;default:0"`
	PaidAt            *time.Time     `json:"paid_at,omitempty" gorm:"type:timestamp"`
	PaymentMethod     *string        `json:"payment_method,omitempty" gorm:"type:varchar(50)"`
	Status            string         `json:"status" gorm:"type:varchar(20);not null;default:'pending';index"`
	LateFee           int64          `json:"late_fee" gorm:"type:bigint;not null;default:0"`
	TransactionID     *string        `json:"transaction_id,omitempty" gorm:"type:uuid;index"`
	CreatedAt         time.Time      `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt         time.Time      `json:"updated_at" gorm:"autoUpdateTime;not null"`
	DeletedAt         gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	Loan        Loan                      `gorm:"foreignKey:LoanID"`
	User        *users.User               `gorm:"foreignKey:UserID"`
	Transaction *transactions.Transaction `gorm:"foreignKey:TransactionID"`
}

// TableName specifies the table name for Repayment model
func (Repayment) TableName() string {
	return "repayments"
}

// BeforeCreate sets the ID before creating a new repayment
func (r *Repayment) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	r.ID = id.String()
	return nil
}

const (
	// Interest type constants
	InterestTypeSimple   = "simple"
	InterestTypeCompound = "compound"

	// Loan Status
	LoanStatusPending    = "pending"
	LoanStatusApproved   = "approved"
	LoanStatusDisbursing = "disbursing"
	LoanStatusDisbursed  = "disbursed"
	LoanStatusRepaid     = "repaid"
	LoanStatusDefaulted  = "defaulted"
	LoanStatusCancelled  = "cancelled"
	// LoanStatusOffRampFailed marks a loan whose vault borrow succeeded but
	// whose fiat disbursement could not be completed. The borrowed USDC has
	// been (or will be) repaid to the vault; the borrower owes nothing and
	// did not default. Distinct from LoanStatusDefaulted, which is a
	// borrower-side credit event.
	LoanStatusOffRampFailed = "offramp_failed"

	// Repayment Status
	RepaymentStatusPending = "pending"
	RepaymentStatusPaid    = "paid"
	RepaymentStatusOverdue = "overdue"
	RepaymentStatusPartial = "partial"
	RepaymentStatusWaived  = "waived"

	// Disbursement Status (off-ramp lifecycle)
	DisbursementStatusPending    = "pending"
	DisbursementStatusProcessing = "processing"
	DisbursementStatusCompleted  = "completed"
	DisbursementStatusFailed     = "failed"

	// Vault Repay Status — tracks treasury→vault USDC return. NULL when no
	// repay has been attempted (e.g. direct settlement still in flight, or
	// completed direct where USDC went to YC and no repay is owed).
	VaultRepayStatusSuccess = "success"
	VaultRepayStatusFailed  = "failed"
)
