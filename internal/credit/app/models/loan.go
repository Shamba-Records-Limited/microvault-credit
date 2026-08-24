package models

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	users "github.com/Shamba-Records-Limited/microvault/pkg/models"
)

// Loan represents a loan record
// Amounts stored in smallest unit, rates in basis points
type Loan struct {
	ID              string  `json:"id" gorm:"type:uuid;primaryKey"`
	LoanReference   *string `json:"loan_reference,omitempty" gorm:"column:loan_reference;type:varchar(50);uniqueIndex"`
	UserID          string  `json:"user_id" gorm:"type:uuid;not null;index"`
	AccountID       string  `json:"account_id" gorm:"type:uuid;not null;index"`
	ProductID       *string `json:"product_id,omitempty" gorm:"type:uuid;index"`
	PrincipalAmount int64   `json:"principal_amount" gorm:"type:bigint;not null"`
	PrincipalAsset  string  `json:"principal_asset" gorm:"type:varchar(20);not null;index"`
	// VaultAPRBps is the annual rate that applied when the loan opened, read
	// from the vault at borrow time. It is an audit record of the rate, not a
	// fixed obligation: real accrual is derived from BorrowIndex, and the
	// borrower is told to check their balance rather than quoted a total.
	VaultAPRBps       int32      `json:"vault_apr_bps" gorm:"column:vault_apr_bps;type:int;not null"`
	OriginationFee    *int64     `json:"origination_fee,omitempty" gorm:"type:bigint"`
	OriginationFeeBps *int32     `json:"origination_fee_bps,omitempty" gorm:"type:int"`
	DurationDays      int        `json:"duration_days" gorm:"type:int;not null"`
	RepaymentSchedule string     `json:"repayment_schedule" gorm:"type:varchar(20);not null"`
	DueDate           *time.Time `json:"due_date,omitempty" gorm:"type:timestamp;index"`
	Status            string     `json:"status" gorm:"type:varchar(20);not null;default:'pending';index"`
	ApprovedAt        *time.Time `json:"approved_at,omitempty" gorm:"type:timestamp"`
	ApprovedBy        *string    `json:"approved_by,omitempty" gorm:"type:uuid"`
	DisbursedAt       *time.Time `json:"disbursed_at,omitempty" gorm:"type:timestamp;index"`
	RepaidAt          *time.Time `json:"repaid_at,omitempty" gorm:"type:timestamp"`
	DefaultedAt       *time.Time `json:"defaulted_at,omitempty" gorm:"type:timestamp"`
	VaultTxHash       *string    `json:"vault_tx_hash,omitempty" gorm:"type:varchar(64);index"`
	VaultTxStatus     *string    `json:"vault_tx_status,omitempty" gorm:"type:varchar(20)"`
	VaultRepayTxHash  *string    `json:"vault_repay_tx_hash,omitempty" gorm:"type:varchar(64);index"`
	VaultRepayStatus  *string    `json:"vault_repay_status,omitempty" gorm:"type:varchar(20);index"`
	RampProvider      *string    `json:"ramp_provider,omitempty" gorm:"type:varchar(50)"`
	RampRequestID     *string    `json:"ramp_request_id,omitempty" gorm:"type:varchar(100);index"`
	RampFiatAmount    *int64     `json:"ramp_fiat_amount,omitempty" gorm:"type:bigint"`
	RampFiatCurr      *string    `json:"ramp_fiat_currency,omitempty" gorm:"column:ramp_fiat_currency;type:varchar(10)"`
	SettlementMethod  *string    `json:"settlement_method,omitempty" gorm:"type:varchar(20)"`
	RampSequenceID    *string    `json:"ramp_sequence_id,omitempty" gorm:"type:varchar(200);index"`
	// DisbursementRate is the FX rate actually executed at disbursement, the
	// counterpart to EntryRateBuffered at quote time.
	//
	// Stored as the rate scaled by [RateScaleE8]: 128.23 KES/USD is
	// 12823000000. The name carries no scale suffix and the BIGINT column type
	// does not imply one, so this comment and [RateScaleE8] are the only record
	// of it — always cross the boundary through [RateE8] and [RateFromE8]
	// rather than dividing by hand. An FX rate is not a percentage; do not
	// reach for basis points here (see migration 000019).
	DisbursementRate     *int64 `json:"disbursement_rate,omitempty" gorm:"column:disbursement_rate;type:bigint"`
	DeliveredAmountLocal *int64 `json:"delivered_amount_local,omitempty" gorm:"column:delivered_amount_local;type:bigint"`
	ConversionSpreadBps  *int32 `json:"conversion_spread_bps,omitempty" gorm:"type:int"`
	BorrowIndex          *int64 `json:"borrow_index,omitempty" gorm:"type:bigint"`
	ServiceFeeUSD        *int64 `json:"service_fee_usd,omitempty" gorm:"type:bigint"`
	ServiceFeeLocal      *int64 `json:"service_fee_local,omitempty" gorm:"type:bigint"`
	PartnerFeeUSD        *int64 `json:"partner_fee_usd,omitempty" gorm:"type:bigint"`
	PartnerFeeLocal      *int64 `json:"partner_fee_local,omitempty" gorm:"type:bigint"`

	// Disclosed fees: fixed at creation and never rewritten, unlike the
	// ServiceFee/PartnerFee pair above, which the ramp reports after
	// settlement. Keeping the two families apart preserves what the borrower
	// was told even after the provider reports what it actually charged.
	TelcoFeeUSD   *int64 `json:"telco_fee_usd,omitempty" gorm:"type:bigint"`
	TelcoFeeLocal *int64 `json:"telco_fee_local,omitempty" gorm:"type:bigint"`
	TaxUSD        *int64 `json:"tax_usd,omitempty" gorm:"type:bigint"`
	TaxLocal      *int64 `json:"tax_local,omitempty" gorm:"type:bigint"`

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
	// RampShortCode maps a /r/{code} SMS redirect to RampInteractiveURL.
	RampShortCode *string `json:"ramp_short_code,omitempty" gorm:"type:varchar(24);uniqueIndex"`
	// RampShortCodeExpiresAt bounds the interactive code's life. It is an
	// unauthenticated bearer token, and the disbursement-status gate alone
	// leaves it valid indefinitely on a loan that never reaches a terminal
	// state. Nil on rows predating migration 000024: status gate only.
	RampShortCodeExpiresAt *time.Time `json:"ramp_short_code_expires_at,omitempty" gorm:"type:timestamptz"`
	// RampMoreInfoShortCode maps a /r/{code} SMS redirect to RampMoreInfoURL.
	// Separate from RampShortCode because the two links have opposite
	// lifetimes: the interactive URL must die once the withdrawal settles, and
	// the support link only becomes useful then.
	RampMoreInfoShortCode *string `json:"ramp_more_info_short_code,omitempty" gorm:"type:varchar(24);uniqueIndex"`
	// RampStellarTxHash is the treasury -> MG USDC payment hash. Set once the
	// send succeeds and used as the poller's local idempotency marker, so a
	// slow MG stellar_transaction_id echo can't trigger a duplicate payment.
	RampStellarTxHash *string `json:"ramp_stellar_tx_hash,omitempty" gorm:"type:varchar(64)"`

	// FX audit fields capture the rate the loan was quoted at, the source
	// label, the entry buffer percentage applied, and the user's originally
	// requested local amount — used by the poller's drift detection.
	// EntryRateBuffered already has EntryBufferBps deducted; it is not the
	// raw provider rate, and will not match a live quote from EntryRateSource.
	// Scaled by [RateScaleE8], as DisbursementRate — same caveat about the
	// scale living in the comment rather than the name or column type.
	EntryRateBuffered *int64  `json:"entry_rate_buffered,omitempty" gorm:"column:entry_rate_buffered;type:bigint"`
	EntryRateSource   *string `json:"entry_rate_source,omitempty" gorm:"type:varchar(40)"`
	// EntryBufferBps is a true percentage and so is held in basis points:
	// 1 % is 100. Unlike the rates above, bps is the correct unit here.
	EntryBufferBps       *int32 `json:"entry_buffer_bps,omitempty" gorm:"column:entry_buffer_bps;type:int"`
	RequestedLocalAmount *int64 `json:"requested_local_amount,omitempty" gorm:"type:bigint"`

	// SEP-24 withdraw memo returned on MG's transaction object; used to match
	// refund inbound USDC back to the loan.
	RampWithdrawMemo     *string `json:"ramp_withdraw_memo,omitempty" gorm:"type:varchar(64);index"`
	RampWithdrawMemoType *string `json:"ramp_withdraw_memo_type,omitempty" gorm:"type:varchar(10)"`

	// RampRefundDeclaredAt is set when the anchor first reports a refund, and
	// stands until the inbound USDC is verified on-ledger. No transaction row
	// exists during that window by design — the refund row is written only
	// after the payment is confirmed — so this is the only durable record that
	// a refund is outstanding.
	RampRefundDeclaredAt *time.Time `json:"ramp_refund_declared_at,omitempty" gorm:"type:timestamptz"`

	// RampPickupReadyAt marks that the cash became collectable and the borrower
	// was told. Written before the SMS, so a failed send is not retried on
	// every poll tick. Not derivable from anything on-chain: it records a
	// notification, not a movement of money.
	RampPickupReadyAt *time.Time `json:"ramp_pickup_ready_at,omitempty" gorm:"type:timestamptz"`

	// Refund settlement, written when MoneyGram returns the USDC — usually
	// because the borrower cancelled in MG's UI. RampRefundShortfall is what MG
	// kept back (its refund fee included); non-zero means the treasury absorbed
	// the difference and the loan needs manual settlement.
	RampRefundTxHash    *string    `json:"ramp_refund_tx_hash,omitempty" gorm:"type:varchar(64)"`
	RampRefundAmount    *int64     `json:"ramp_refund_amount,omitempty" gorm:"type:bigint"`
	RampRefundShortfall *int64     `json:"ramp_refund_shortfall,omitempty" gorm:"type:bigint"`
	RampRefundedAt      *time.Time `json:"ramp_refunded_at,omitempty"`

	// Borrower-initiated repayment, tracked apart from both loans.status and
	// the VaultRepay* pair above — those mean "a disbursement was unwound",
	// which is the opposite movement of money.
	//
	// RepaymentStatus and loans.status are allowed to disagree for a window:
	// once cash lands on the treasury the borrower is told immediately, while
	// the treasury-to-vault leg may still be retrying. Status is disbursed and
	// RepaymentStatus is funds_received throughout that window.
	//
	// RepaymentPayoffStroops is quote-locked at initiation, so borrow-index
	// movement afterwards does not change what the borrower owes.
	// RepaymentMGTxID is MoneyGram's transaction ID and the idempotency key
	// for the whole rail; it is uniquely indexed.
	RepaymentStatus        string     `json:"repayment_status" gorm:"type:varchar(20);not null;default:'none'"`
	RepaymentPayoffStroops *int64     `json:"repayment_payoff_stroops,omitempty" gorm:"type:bigint"`
	RepaymentLockedAt      *time.Time `json:"repayment_locked_at,omitempty" gorm:"type:timestamptz"`
	RepaymentExpiresAt     *time.Time `json:"repayment_expires_at,omitempty" gorm:"type:timestamptz"`
	RepaymentMGTxID        *string    `json:"repayment_mg_tx_id,omitempty" gorm:"column:repayment_mg_tx_id;type:varchar(100);uniqueIndex"`
	RepaymentNextPollAt    *time.Time `json:"repayment_next_poll_at,omitempty" gorm:"type:timestamptz;index"`
	// RepaymentReminderSentAt is written before the pre-expiry SMS, so a
	// failing send is not retried on every poll tick. It records a
	// notification rather than a movement of money, so nothing else on the
	// row can stand in for it.
	RepaymentReminderSentAt *time.Time `json:"repayment_reminder_sent_at,omitempty" gorm:"type:timestamptz"`
	// RepaymentVaultTxHash is the treasury-to-vault repay_for transaction.
	// Distinct from VaultRepayTxHash, which means the disbursement was
	// unwound; a borrower settling their debt must not overwrite that.
	RepaymentVaultTxHash *string `json:"repayment_vault_tx_hash,omitempty" gorm:"type:varchar(64)"`
	// RepaymentVaultAttempts counts failed treasury-to-vault repay_for calls.
	// Durable rather than in-memory: a restart must not reset it, or the
	// escalation ceiling would never be reached. Doubles as the escalation
	// marker, since crossing the ceiling happens exactly once.
	RepaymentVaultAttempts int `json:"repayment_vault_attempts" gorm:"not null;default:0"`

	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"autoUpdateTime;not null"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	User    *users.User    `gorm:"foreignKey:UserID"`
	Account *users.Account `gorm:"foreignKey:AccountID"`
	Product *LoanProduct   `gorm:"foreignKey:ProductID"`
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

	// Disbursement status vocabulary. No longer a column: these are derived
	// from loans.status, ramp_refund_declared_at and ramp_pickup_ready_at, and
	// exist so the MoneyGram poller's state machine has stable names to
	// compare against. See DeriveDisbursementStatus.
	//
	// The refund and MoneyGram states were written by the poller long before
	// they appeared here, so queries built against this enum silently missed
	// them. They are declared now so there is one vocabulary.
	//
	// Note YellowCard's own wire value for completion is "complete"; it is
	// canonicalised to "completed" on the way in, so nothing downstream has to
	// know which provider produced a row.
	DisbursementStatusPending        = "pending"
	DisbursementStatusMGInitiated    = "mg_initiated"
	DisbursementStatusProcessing     = "processing"
	DisbursementStatusCompleted      = "completed"
	DisbursementStatusFailed         = "failed"
	DisbursementStatusRefundPending  = "refund_pending"
	DisbursementStatusRefundReceived = "refund_received"

	// Vault Repay Status — tracks treasury→vault USDC return. NULL when no
	// repay has been attempted (e.g. direct settlement still in flight, or
	// completed direct where USDC went to YC and no repay is owed).
	VaultRepayStatusSuccess = "success"
	VaultRepayStatusFailed  = "failed"

	// Loan Repayment Status — the borrower paying their loan off, stored on
	// loans.repayment_status. The LoanRepayment prefix is deliberate: it keeps
	// these apart from VaultRepayStatus* above, which records a disbursement
	// being unwound rather than a borrower settling a debt.
	//
	// The two live states are initiated (deposit open, borrower has not paid
	// the agent yet) and funds_received (USDC on the treasury, vault leg not
	// yet confirmed). Everything else is terminal.
	LoanRepaymentStatusNone = "none"
	// LoanRepaymentStatusInitiated means a deposit is open and the payoff
	// quote is locked until repayment_expires_at.
	LoanRepaymentStatusInitiated = "initiated"
	// LoanRepaymentStatusFundsReceived means the cash reached the treasury as
	// USDC. The borrower is told at this point, before the vault leg settles.
	LoanRepaymentStatusFundsReceived = "funds_received"
	// LoanRepaymentStatusSettled means the treasury-to-vault leg confirmed.
	// This is the only state that flips loans.status to repaid.
	LoanRepaymentStatusSettled = "settled"
	// LoanRepaymentStatusExpired means the cash-in window elapsed without the
	// borrower paying. The quote lock is released and the loan returns to its
	// prior status; the borrower owes what they owed before.
	LoanRepaymentStatusExpired = "expired"
	// LoanRepaymentStatusFailed means the rail failed before funds moved. It
	// is not used for a failed vault leg — funds already on the treasury stay
	// at funds_received so reconciliation keeps retrying.
	LoanRepaymentStatusFailed = "failed"
)

// IsRepaymentOpen reports whether a borrower repayment is in flight and owned
// by the poller or the reconciliation loop. It matches the predicate of the
// idx_loans_repayment_open index; keep the two in step.
func (l *Loan) IsRepaymentOpen() bool {
	switch l.RepaymentStatus {
	case LoanRepaymentStatusInitiated, LoanRepaymentStatusFundsReceived:
		return true
	default:
		return false
	}
}

// DeriveDisbursementStatus reports where a loan's payout stands.
//
// This replaced a stored column. Every terminal value restates the loan's own
// status, so keeping a second copy only created a way for the two to disagree —
// which they did: MoneyGram wrote "completed" while the reader compared against
// YellowCard's "complete", and no cash-pickup loan ever left disbursing.
//
// The two non-derivable states have their own markers. A declared-but-
// unverified refund has no transaction row by design, and pickup-ready records
// an SMS rather than a movement of money.
func (l *Loan) DeriveDisbursementStatus() string {
	switch {
	case l.Status == LoanStatusCancelled:
		return DisbursementStatusRefundReceived
	case l.Status == LoanStatusOffRampFailed:
		return DisbursementStatusFailed
	case l.Status == LoanStatusDisbursed:
		return DisbursementStatusCompleted
	case l.RampRefundDeclaredAt != nil:
		return DisbursementStatusRefundPending
	case l.RampPickupReadyAt != nil:
		return DisbursementStatusProcessing
	case l.RampProvider != nil && *l.RampProvider == "moneygram":
		return DisbursementStatusMGInitiated
	default:
		return DisbursementStatusPending
	}
}

// IsDisbursementTerminal reports whether a loan's payout has finished, however
// it finished. Terminal loans are not polled and hold no claim on the borrower.
func (l *Loan) IsDisbursementTerminal() bool {
	switch l.Status {
	case LoanStatusDisbursed, LoanStatusOffRampFailed, LoanStatusCancelled,
		LoanStatusRepaid, LoanStatusDefaulted:
		return true
	default:
		return false
	}
}

// RateScaleE8 is the fixed-point scale applied to stored FX rates.
const RateScaleE8 = 100_000_000

// RateE8 converts an FX rate to its stored form, rounding to the nearest unit
// of the 10^8 scale. Returns nil for a non-positive rate so an unknown rate
// stays NULL rather than being recorded as zero.
func RateE8(rate float64) *int64 {
	if rate <= 0 {
		return nil
	}
	v := int64(math.Round(rate * RateScaleE8))
	return &v
}

// RateFromE8 converts a stored rate back to its decimal form. Returns 0 when
// the rate was never recorded.
func RateFromE8(e8 *int64) float64 {
	if e8 == nil {
		return 0
	}
	return float64(*e8) / RateScaleE8
}

// BufferBps converts a buffer fraction (0.01 = 1 %) to basis points. Returns
// nil for a non-positive fraction, leaving an unrecorded buffer NULL.
func BufferBps(fraction float64) *int32 {
	if fraction <= 0 {
		return nil
	}
	v := int32(math.Round(fraction * 10_000))
	return &v
}

// BufferFraction converts stored basis points back to a fraction.
func BufferFraction(bps *int32) float64 {
	if bps == nil {
		return 0
	}
	return float64(*bps) / 10_000
}
