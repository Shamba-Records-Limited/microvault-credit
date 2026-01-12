package models

import (
	"slices"
	"time"

	users "github.com/Shamba-Records-Limited/Microvault/pkg/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// FarmRecord represents agricultural activity records
// Amounts stored in smallest unit
type FarmRecord struct {
	ID          string          `json:"id" gorm:"type:uuid;primaryKey"`
	UserID      string          `json:"user_id" gorm:"type:uuid;not null;index"`
	RecordType  string          `json:"record_type" gorm:"type:varchar(50);not null;index"`
	RecordDate  time.Time       `json:"record_date" gorm:"type:timestamp;not null;index"`
	CropType    *string         `json:"crop_type,omitempty" gorm:"type:varchar(100)"`
	Quantity    *int64          `json:"quantity,omitempty" gorm:"type:bigint"`
	Unit        *string         `json:"unit,omitempty" gorm:"type:varchar(50)"`
	Amount      *int64          `json:"amount,omitempty" gorm:"type:bigint"`
	Description *string         `json:"description,omitempty" gorm:"type:text"`
	DocumentID  *string         `json:"document_id,omitempty" gorm:"type:uuid;index"`
	Verified    bool            `json:"verified" gorm:"type:boolean;not null;default:false"`
	Metadata    *datatypes.JSON `json:"metadata,omitempty" gorm:"type:jsonb"`
	CreatedAt   time.Time       `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt   time.Time       `json:"updated_at" gorm:"autoUpdateTime;not null"`

	User *users.User `gorm:"foreignKey:UserID"`
}

// DateRange represents a date range for filtering and analysis
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// FarmRecordFilter represents filter criteria for querying farm records
type FarmRecordFilter struct {
	UserID     string     `json:"user_id"`
	RecordType string     `json:"record_type,omitempty"`
	CropType   string     `json:"crop_type,omitempty"`
	StartDate  *time.Time `json:"start_date,omitempty"`
	EndDate    *time.Time `json:"end_date,omitempty"`
	Verified   *bool      `json:"verified,omitempty"`
	Limit      int        `json:"limit,omitempty"`
	Offset     int        `json:"offset,omitempty"`
}

// FarmRecordSummary represents comprehensive farm record statistics
// Amounts stored in smallest unit
type FarmRecordSummary struct {
	UserID           string           `json:"user_id"`
	DateRange        DateRange        `json:"date_range"`
	TotalRecords     int64            `json:"total_records"`
	VerifiedRecords  int64            `json:"verified_records"`
	TotalSalesAmount int64            `json:"total_sales_amount"`
	TotalExpenses    int64            `json:"total_expenses"`
	TotalInputCosts  int64            `json:"total_input_costs"`
	CropTypes        []string         `json:"crop_types"`
	RecordsByType    map[string]int64 `json:"records_by_type"`
	MonthlySales     map[string]int64 `json:"monthly_sales"`
}

// TableName specifies the table name for FarmRecord model
func (FarmRecord) TableName() string {
	return "farm_records"
}

// BeforeCreate sets the ID before creating a new farm record
func (fr *FarmRecord) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	fr.ID = id.String()
	return nil
}

// IsValidRecordType checks if the record type is valid
func (f *FarmRecord) IsValidRecordType() bool {
	validTypes := []string{
		FarmRecordTypeHarvest,
		FarmRecordTypeInput,
		FarmRecordTypeSale,
		FarmRecordTypeExpense,
		FarmRecordTypePlanting,
		FarmRecordTypeMaintenance,
	}
	return slices.Contains(validTypes, f.RecordType)
}

// GetAmountValue returns the amount value or 0 if nil
func (f *FarmRecord) GetAmountValue() int64 {
	if f.Amount == nil {
		return 0
	}
	return *f.Amount
}

// GetQuantityValue returns the quantity value or 0 if nil
func (f *FarmRecord) GetQuantityValue() int64 {
	if f.Quantity == nil {
		return 0
	}
	return *f.Quantity
}

// IsIncomeRecord returns true if this record represents income (sale/harvest)
func (f *FarmRecord) IsIncomeRecord() bool {
	return f.RecordType == FarmRecordTypeSale || f.RecordType == FarmRecordTypeHarvest
}

// IsExpenseRecord returns true if this record represents an expense
func (f *FarmRecord) IsExpenseRecord() bool {
	return f.RecordType == FarmRecordTypeExpense || f.RecordType == FarmRecordTypeInput
}

// Farm record type constants
const (
	FarmRecordTypeHarvest     = "harvest"
	FarmRecordTypeInput       = "input"
	FarmRecordTypeSale        = "sale"
	FarmRecordTypeExpense     = "expense"
	FarmRecordTypePlanting    = "planting"
	FarmRecordTypeMaintenance = "maintenance"
)
