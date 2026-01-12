package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreditScoringFactor represents a configurable credit scoring factor
// Weight stored in basis points
type CreditScoringFactor struct {
	ID                string    `json:"id" gorm:"type:uuid;primaryKey"`
	FactorName        string    `json:"factor_name" gorm:"type:varchar(100);uniqueIndex;not null"`
	FactorDescription *string   `json:"factor_description,omitempty" gorm:"type:text"`
	WeightBps         int32     `json:"weight_bps" gorm:"type:int;not null"`
	IsActive          bool      `json:"is_active" gorm:"type:boolean;not null;default:true;index"`
	CalculationMethod *string   `json:"calculation_method,omitempty" gorm:"type:varchar(100)"`
	MinDataRequired   int       `json:"min_data_required" gorm:"type:int;not null;default:1"`
	CreatedAt         time.Time `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt         time.Time `json:"updated_at" gorm:"autoUpdateTime;not null"`
	CreatedBy         *string   `json:"created_by,omitempty" gorm:"type:uuid"`
	UpdatedBy         *string   `json:"updated_by,omitempty" gorm:"type:uuid"`
}

// TableName specifies the table name for CreditScoringFactor model
func (CreditScoringFactor) TableName() string {
	return "credit_scoring_factors"
}

// BeforeCreate sets the ID before creating a new credit scoring factor
func (csf *CreditScoringFactor) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	csf.ID = id.String()
	return nil
}

// RiskTierConfig represents a risk tier configuration
type RiskTierConfig struct {
	ID          string    `json:"id" gorm:"type:uuid;primaryKey"`
	TierName    string    `json:"tier_name" gorm:"type:varchar(50);uniqueIndex;not null"`
	MinScore    int       `json:"min_score" gorm:"type:int;not null"`
	MaxScore    int       `json:"max_score" gorm:"type:int;not null"`
	TierOrder   int       `json:"tier_order" gorm:"type:int;not null;uniqueIndex"`
	Description *string   `json:"description,omitempty" gorm:"type:text"`
	ColorCode   *string   `json:"color_code,omitempty" gorm:"type:varchar(20)"`
	IsActive    bool      `json:"is_active" gorm:"type:boolean;not null;default:true;index"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime;not null"`
	CreatedBy   *string   `json:"created_by,omitempty" gorm:"type:uuid"`
	UpdatedBy   *string   `json:"updated_by,omitempty" gorm:"type:uuid"`
}

// TableName specifies the table name for RiskTierConfig model
func (RiskTierConfig) TableName() string {
	return "risk_tier_configs"
}

// BeforeCreate sets the ID before creating a new risk tier config
func (rtc *RiskTierConfig) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	rtc.ID = id.String()
	return nil
}

// LoanLimitConfig represents loan limits for a risk tier
// Amounts stored in smallest unit, rates in basis points
type LoanLimitConfig struct {
	ID                  string    `json:"id" gorm:"type:uuid;primaryKey"`
	RiskTier            string    `json:"risk_tier" gorm:"type:varchar(50);uniqueIndex;not null"`
	MinLoanAmount       int64     `json:"min_loan_amount" gorm:"type:bigint;not null"`
	MaxLoanAmount       int64     `json:"max_loan_amount" gorm:"type:bigint;not null"`
	IncomeMultiplierBps int32     `json:"income_multiplier_bps" gorm:"type:int;not null"`
	MaxConcurrentLoans  int       `json:"max_concurrent_loans" gorm:"type:int;not null"`
	MaxLoanDurationDays int       `json:"max_loan_duration_days" gorm:"type:int;not null"`
	InterestRateBps     int32     `json:"interest_rate_bps" gorm:"type:int;not null"`
	LateFeeBps          *int32    `json:"late_fee_bps,omitempty" gorm:"type:int"`
	DefaultPenaltyBps   *int32    `json:"default_penalty_bps,omitempty" gorm:"type:int"`
	IsActive            bool      `json:"is_active" gorm:"type:boolean;not null;default:true;index"`
	CreatedAt           time.Time `json:"created_at" gorm:"autoCreateTime;not null"`
	CreatedBy           *string   `json:"created_by,omitempty" gorm:"type:uuid"`
	UpdatedAt           time.Time `json:"updated_at" gorm:"autoUpdateTime;not null"`
	UpdatedBy           *string   `json:"updated_by,omitempty" gorm:"type:uuid"`
}

// TableName specifies the table name for LoanLimitConfig model
func (LoanLimitConfig) TableName() string {
	return "loan_limit_configs"
}

// BeforeCreate sets the ID before creating a new loan limit config
func (llc *LoanLimitConfig) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	llc.ID = id.String()
	return nil
}

// GlobalLendingLimit represents a global platform configuration
type GlobalLendingLimit struct {
	ID          string    `json:"id" gorm:"type:uuid;primaryKey"`
	ConfigKey   string    `json:"config_key" gorm:"type:varchar(100);uniqueIndex;not null"`
	ConfigValue string    `json:"config_value" gorm:"type:varchar(255);not null"`
	ValueType   string    `json:"value_type" gorm:"type:varchar(20);not null;index"`
	Description *string   `json:"description,omitempty" gorm:"type:text"`
	Category    *string   `json:"category,omitempty" gorm:"type:varchar(50);index"`
	IsActive    bool      `json:"is_active" gorm:"type:boolean;not null;default:true;index"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime;not null"`
	CreatedBy   *string   `json:"created_by,omitempty" gorm:"type:uuid"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime;not null"`
	UpdatedBy   *string   `json:"updated_by,omitempty" gorm:"type:uuid"`
}

// TableName specifies the table name for GlobalLendingLimit model
func (GlobalLendingLimit) TableName() string {
	return "global_lending_limits"
}

// BeforeCreate sets the ID before creating a new global lending limit
func (gll *GlobalLendingLimit) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	gll.ID = id.String()
	return nil
}

// CreditConfigAuditLog represents an audit log entry
type CreditConfigAuditLog struct {
	ID           string         `json:"id" gorm:"type:uuid;primaryKey"`
	ConfigTable  string         `json:"config_table" gorm:"type:varchar(100);not null;index"`
	ConfigID     string         `json:"config_id" gorm:"type:uuid;not null;index"`
	Action       string         `json:"action" gorm:"type:varchar(20);not null;index"`
	OldValues    map[string]any `json:"old_values,omitempty" gorm:"type:jsonb"`
	NewValues    map[string]any `json:"new_values,omitempty" gorm:"type:jsonb"`
	ChangedBy    *string        `json:"changed_by,omitempty" gorm:"type:uuid"`
	ChangeReason *string        `json:"change_reason,omitempty" gorm:"type:text"`
	CreatedAt    time.Time      `json:"created_at" gorm:"autoCreateTime;not null;index"`
}

// TableName specifies the table name for CreditConfigAuditLog model
func (CreditConfigAuditLog) TableName() string {
	return "credit_config_audit_logs"
}

// BeforeCreate sets the ID before creating a new credit config audit log
func (ccal *CreditConfigAuditLog) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	ccal.ID = id.String()
	return nil
}

// Config value type constants
const (
	ConfigValueTypeInteger = "integer"
	ConfigValueTypeDecimal = "decimal"
	ConfigValueTypeBoolean = "boolean"
	ConfigValueTypeString  = "string"
)

// Config category constants
const (
	ConfigCategoryLimits      = "limits"
	ConfigCategoryFees        = "fees"
	ConfigCategoryScoring     = "scoring"
	ConfigCategoryOperational = "operational"
)

// Audit action constants
const (
	AuditActionCreate     = "create"
	AuditActionUpdate     = "update"
	AuditActionDelete     = "delete"
	AuditActionActivate   = "activate"
	AuditActionDeactivate = "deactivate"
)
