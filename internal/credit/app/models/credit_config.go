package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

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
	ConfigCategoryOperational = "operational"
)
