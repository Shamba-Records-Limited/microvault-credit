package models

import (
	"time"

	users "github.com/Shamba-Records-Limited/Microvault/pkg/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreditFactor represents individual credit scoring factors
// Values and weights stored in basis points
type CreditFactor struct {
	ID                string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID            string     `json:"user_id" gorm:"type:uuid;not null;index"`
	CreditScoreID     *string    `json:"credit_score_id,omitempty" gorm:"type:uuid;index"`
	FactorName        string     `json:"factor_name" gorm:"type:varchar(100);not null;index"`
	FactorValueBps    int32      `json:"factor_value_bps" gorm:"type:int;not null"`
	FactorWeightBps   int32      `json:"factor_weight_bps" gorm:"type:int;not null"`
	WeightedScoreBps  int32      `json:"weighted_score_bps" gorm:"type:int;not null"`
	CalculationMethod *string    `json:"calculation_method,omitempty" gorm:"type:varchar(100)"`
	DataSource        *string    `json:"data_source,omitempty" gorm:"type:varchar(50)"`
	CalculatedAt      time.Time  `json:"calculated_at" gorm:"type:timestamp;not null;index"`
	CreatedAt         time.Time  `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt         time.Time  `json:"updated_at" gorm:"autoUpdateTime;not null"`
	DeletedAt         *time.Time `json:"deleted_at" gorm:"type:timestamp"`

	User        *users.User  `gorm:"foreignKey:UserID"`
	CreditScore *CreditScore `gorm:"foreignKey:CreditScoreID"`
}

// TableName specifies the table name for CreditFactor model
func (CreditFactor) TableName() string {
	return "credit_factors"
}

// BeforeCreate sets the ID before creating a new credit factor
func (cf *CreditFactor) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	cf.ID = id.String()
	return nil
}

// CalculateWeightedScore computes and sets the weighted score in basis points
func (cf *CreditFactor) CalculateWeightedScore() {
	cf.WeightedScoreBps = (cf.FactorValueBps * cf.FactorWeightBps) / 10000
}

// IsValid checks if factor values are within acceptable ranges
// FactorValue should be 0-10000 bps (0-100%), FactorWeight should be 0-10000 bps (0-100%)
func (cf *CreditFactor) IsValid() bool {
	return cf.FactorValueBps >= 0 && cf.FactorValueBps <= 10000 &&
		cf.FactorWeightBps >= 0 && cf.FactorWeightBps <= 10000
}

// GetContributionPercentage returns this factor's contribution to the total score in basis points
func (cf *CreditFactor) GetContributionPercentage(totalWeightedScoreBps int32) int32 {
	if totalWeightedScoreBps == 0 {
		return 0
	}
	return (cf.WeightedScoreBps * 10000) / totalWeightedScoreBps
}

// Known credit factor names
const (
	FactorNameRepaymentHistory       = "repayment_history"
	FactorNameIncomeStability        = "income_stability"
	FactorNameCashflowHealth         = "cashflow_health"
	FactorNameTransactionConsistency = "transaction_consistency"
	FactorNameAgriculturalActivity   = "agricultural_activity"
)

// Data source constants
const (
	DataSourceLoansTable            = "loans_table"
	DataSourceTransactionsExtracted = "transactions_extracted"
	DataSourceCashflowAnalysis      = "cashflow_analysis"
	DataSourceFarmRecords           = "farm_records"
)
