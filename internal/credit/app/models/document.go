package models

import (
	"time"

	users "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// UserDocument represents an uploaded document
type UserDocument struct {
	ID               string          `json:"id" gorm:"type:uuid;primaryKey"`
	UserID           string          `json:"user_id" gorm:"type:uuid;not null;index"`
	DocumentType     string          `json:"document_type" gorm:"type:varchar(50);not null;index"`
	FileName         string          `json:"file_name" gorm:"type:varchar(255);not null"`
	FileSize         int             `json:"file_size" gorm:"type:int;not null"`
	MimeType         string          `json:"mime_type" gorm:"type:varchar(100);not null"`
	StoragePath      string          `json:"storage_path" gorm:"type:varchar(500);not null"`
	UploadDate       time.Time       `json:"upload_date" gorm:"type:timestamp;not null"`
	ProcessingStatus string          `json:"processing_status" gorm:"type:varchar(20);not null;default:'pending';index"`
	ParsedData       *datatypes.JSON `json:"parsed_data,omitempty" gorm:"type:jsonb"`
	ErrorMessage     *string         `json:"error_message,omitempty" gorm:"type:text"`
	Metadata         *datatypes.JSON `json:"metadata,omitempty" gorm:"type:jsonb"`
	CreatedAt        time.Time       `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt        time.Time       `json:"updated_at" gorm:"autoUpdateTime;not null"`

	User *users.User `gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for UserDocument model
func (UserDocument) TableName() string {
	return "user_documents"
}

// BeforeCreate sets the ID before creating a new user document
func (doc *UserDocument) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	doc.ID = id.String()
	return nil
}

// TransactionExtracted represents a transaction parsed from statements
// Amounts stored in smallest unit
type TransactionExtracted struct {
	ID              string         `json:"id" gorm:"type:uuid;primaryKey"`
	DocumentID      string         `json:"document_id" gorm:"type:uuid;not null;index"`
	UserID          string         `json:"user_id" gorm:"type:uuid;not null;index"`
	TransactionDate time.Time      `json:"transaction_date" gorm:"type:timestamp;not null;index"`
	TransactionType string         `json:"transaction_type" gorm:"type:varchar(20);not null;index"`
	Amount          int64          `json:"amount" gorm:"type:bigint;not null"`
	BalanceAfter    *int64         `json:"balance_after,omitempty" gorm:"type:bigint"`
	Counterparty    *string        `json:"counterparty,omitempty" gorm:"type:varchar(255)"`
	Description     *string        `json:"description,omitempty" gorm:"type:text"`
	Category        *string        `json:"category,omitempty" gorm:"type:varchar(50);index"`
	Source          string         `json:"source" gorm:"type:varchar(20);not null;index"`
	CreatedAt       time.Time      `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt       time.Time      `json:"updated_at" gorm:"autoUpdateTime;not null"`
	DeletedAt       gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	Document *UserDocument `gorm:"foreignKey:DocumentID"`
	User     *users.User   `gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for TransactionExtracted model
func (TransactionExtracted) TableName() string {
	return "transactions_extracted"
}

// BeforeCreate sets the ID before creating a new transaction extracted
func (tx *TransactionExtracted) BeforeCreate(db *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	tx.ID = id.String()
	return nil
}

// Document type constants
const (
	DocumentTypeMPesaStatement   = "mpesa_statement"
	DocumentTypeBankStatement    = "bank_statement"
	DocumentTypeFarmRecord       = "farm_record"
	DocumentTypeCollectionRecord = "collection_record"
	DocumentTypeIncomeProof      = "income_proof"
	DocumentTypeOther            = "other"
)

// Processing status constants
const (
	ProcessingStatusPending    = "pending"
	ProcessingStatusProcessing = "processing"
	ProcessingStatusCompleted  = "completed"
	ProcessingStatusFailed     = "failed"
	ProcessingStatusVerified   = "verified"
)

// Transaction type constants
const (
	TxTypeReceive  = "receive"
	TxTypeSend     = "send"
	TxTypeWithdraw = "withdraw"
	TxTypeDeposit  = "deposit"
	TxTypeAirtime  = "airtime"
	TxTypeTransfer = "transfer"
)

// Transaction source constants
const (
	TxSourceMPesa  = "mpesa"
	TxSourceBank   = "bank"
	TxSourceManual = "manual"
	TxSourceAPI    = "api"
)

// Risk tier constants (adding to existing credit score constants)
const (
	RiskTierExcellent = "excellent"
	RiskTierGood      = "good"
	RiskTierFair      = "fair"
	RiskTierPoor      = "poor"
	RiskTierHighRisk  = "high_risk"
)
