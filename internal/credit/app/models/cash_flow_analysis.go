package models

import (
	"encoding/json"
	"time"

	users "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// IncomeSource represents a single income source
// Amounts stored in smallest unit, percentages in basis points
type IncomeSource struct {
	Name       string `json:"name"`
	Amount     int64  `json:"amount"`
	Frequency  string `json:"frequency"`
	Percentage int32  `json:"percentage_bps"`
}

// ExpenseCategory represents a spending category
// Amounts stored in smallest unit, percentages in basis points
type ExpenseCategory struct {
	Category   string `json:"category"`
	Amount     int64  `json:"amount"`
	Percentage int32  `json:"percentage_bps"`
}

// SeasonalPattern represents seasonal cashflow trends
// Amounts stored in smallest unit
type SeasonalPattern struct {
	Month       int   `json:"month"`
	AvgIncome   int64 `json:"avg_income"`
	AvgExpenses int64 `json:"avg_expenses"`
	NetCashflow int64 `json:"net_cashflow"`
}

// RiskFlag represents identified financial risks
type RiskFlag struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"` // low, medium, high
	Description string `json:"description"`
}

// CashflowAnalysis represents cashflow analysis for a user
// Amounts stored in smallest unit, rates in basis points
type CashflowAnalysis struct {
	ID                     string          `json:"id" gorm:"type:uuid;primaryKey"`
	UserID                 string          `json:"user_id" gorm:"type:uuid;not null;index"`
	AnalysisPeriodStart    time.Time       `json:"analysis_period_start" gorm:"type:timestamp;not null;index"`
	AnalysisPeriodEnd      time.Time       `json:"analysis_period_end" gorm:"type:timestamp;not null;index"`
	TotalIncome            int64           `json:"total_income" gorm:"type:bigint;not null;default:0"`
	TotalExpenses          int64           `json:"total_expenses" gorm:"type:bigint;not null;default:0"`
	NetCashflow            int64           `json:"net_cashflow" gorm:"type:bigint;not null;default:0"`
	AvgMonthlyIncome       int64           `json:"avg_monthly_income" gorm:"type:bigint;not null;default:0"`
	AvgMonthlyExpenses     int64           `json:"avg_monthly_expenses" gorm:"type:bigint;not null;default:0"`
	IncomeStabilityScore   int             `json:"income_stability_score" gorm:"type:int;not null;default:0"`
	ExpenseRegularityScore int             `json:"expense_regularity_score" gorm:"type:int;not null;default:0"`
	SavingsRateBps         int32           `json:"savings_rate_bps" gorm:"type:int;not null;default:0"`
	TransactionCount       int             `json:"transaction_count" gorm:"type:int;not null;default:0"`
	UniqueIncomeSources    int             `json:"unique_income_sources" gorm:"type:int;not null;default:0"`
	IncomeSources          *datatypes.JSON `json:"income_sources,omitempty" gorm:"type:jsonb"`
	ExpenseCategories      *datatypes.JSON `json:"expense_categories,omitempty" gorm:"type:jsonb"`
	SeasonalPattern        *datatypes.JSON `json:"seasonal_pattern,omitempty" gorm:"type:jsonb"`
	RiskFlags              *datatypes.JSON `json:"risk_flags,omitempty" gorm:"type:jsonb"`
	CalculatedAt           time.Time       `json:"calculated_at" gorm:"type:timestamp;not null"`
	CreatedAt              time.Time       `json:"created_at" gorm:"autoCreateTime;not null"`
	UpdatedAt              time.Time       `json:"updated_at" gorm:"autoUpdateTime;not null"`
	DeletedAt              gorm.DeletedAt  `json:"deleted_at" gorm:"index"`

	User *users.User `gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for CashflowAnalysis model
func (CashflowAnalysis) TableName() string {
	return "cashflow_analyses"
}

// BeforeCreate sets the ID before creating a new cashflow analysis
func (ca *CashflowAnalysis) BeforeCreate(tx *gorm.DB) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	ca.ID = id.String()
	return nil
}

// --- Calculation Helpers ---
//
// CalculateNetCashflow computes and sets net cashflow
func (c *CashflowAnalysis) CalculateNetCashflow() {
	c.NetCashflow = c.TotalIncome - c.TotalExpenses
}

// CalculateSavingsRate computes and sets the savings rate in basis points
func (c *CashflowAnalysis) CalculateSavingsRate() {
	if c.TotalIncome == 0 {
		c.SavingsRateBps = 0
		return
	}
	c.SavingsRateBps = int32((c.TotalIncome - c.TotalExpenses) * 10000 / c.TotalIncome)
}

// CalculateMonthlyAverages computes average monthly income and expenses
func (c *CashflowAnalysis) CalculateMonthlyAverages() {
	months := c.GetAnalysisPeriodMonths()
	if months == 0 {
		return
	}
	c.AvgMonthlyIncome = c.TotalIncome / int64(months)
	c.AvgMonthlyExpenses = c.TotalExpenses / int64(months)
}

// --- Period Helpers ---
//
// GetAnalysisPeriodMonths returns the number of months in the analysis period
func (c *CashflowAnalysis) GetAnalysisPeriodMonths() int {
	if c.AnalysisPeriodEnd.Before(c.AnalysisPeriodStart) {
		return 0
	}
	years := c.AnalysisPeriodEnd.Year() - c.AnalysisPeriodStart.Year()
	months := int(c.AnalysisPeriodEnd.Month()) - int(c.AnalysisPeriodStart.Month())
	total := years*12 + months
	if total < 1 {
		return 1
	}
	return total
}

// GetAnalysisPeriodDays returns the number of days in the analysis period
func (c *CashflowAnalysis) GetAnalysisPeriodDays() int {
	return int(c.AnalysisPeriodEnd.Sub(c.AnalysisPeriodStart).Hours() / 24)
}

// IsStale checks if the analysis is older than the given duration
func (c *CashflowAnalysis) IsStale(maxAge time.Duration) bool {
	return time.Since(c.CalculatedAt) > maxAge
}

// CoversPeriod checks if the analysis covers a specific date
func (c *CashflowAnalysis) CoversPeriod(date time.Time) bool {
	return !date.Before(c.AnalysisPeriodStart) && !date.After(c.AnalysisPeriodEnd)
}

// --- Validation Helpers ---
//
// IsValid validates the analysis data
func (c *CashflowAnalysis) IsValid() bool {
	return c.UserID != "" &&
		!c.AnalysisPeriodStart.IsZero() &&
		!c.AnalysisPeriodEnd.IsZero() &&
		c.AnalysisPeriodEnd.After(c.AnalysisPeriodStart) &&
		c.TotalIncome >= 0 &&
		c.TotalExpenses >= 0 &&
		c.IncomeStabilityScore >= 0 && c.IncomeStabilityScore <= 100 &&
		c.ExpenseRegularityScore >= 0 && c.ExpenseRegularityScore <= 100
}

// --- Financial Health Helpers ---
//
// IsPositiveCashflow returns true if income exceeds expenses
func (c *CashflowAnalysis) IsPositiveCashflow() bool {
	return c.NetCashflow > 0
}

// GetCashflowHealthStatus returns a health rating based on savings rate in basis points
func (c *CashflowAnalysis) GetCashflowHealthStatus(minSavingsRateBps int32, averageSavingsRateBps int32, maxSavingsRateBps int32) string {
	switch {
	case c.SavingsRateBps >= maxSavingsRateBps:
		return "excellent"
	case c.SavingsRateBps >= averageSavingsRateBps:
		return "good"
	case c.SavingsRateBps >= minSavingsRateBps:
		return "fair"
	default:
		return "poor"
	}
}

// --- JSON Field Helpers ---
//
// GetIncomeSources parses and returns income sources
func (c *CashflowAnalysis) GetIncomeSources() ([]IncomeSource, error) {
	if c.IncomeSources == nil {
		return nil, nil
	}
	var sources []IncomeSource
	if err := json.Unmarshal(*c.IncomeSources, &sources); err != nil {
		return nil, err
	}
	return sources, nil
}

// SetIncomeSources sets income sources from a slice
func (c *CashflowAnalysis) SetIncomeSources(sources []IncomeSource) error {
	data, err := json.Marshal(sources)
	if err != nil {
		return err
	}
	jsonData := datatypes.JSON(data)
	c.IncomeSources = &jsonData
	return nil
}

// GetExpenseCategories parses and returns expense categories
func (c *CashflowAnalysis) GetExpenseCategories() ([]ExpenseCategory, error) {
	if c.ExpenseCategories == nil {
		return nil, nil
	}
	var categories []ExpenseCategory
	if err := json.Unmarshal(*c.ExpenseCategories, &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// SetExpenseCategories sets expense categories from a slice
func (c *CashflowAnalysis) SetExpenseCategories(categories []ExpenseCategory) error {
	data, err := json.Marshal(categories)
	if err != nil {
		return err
	}
	jsonData := datatypes.JSON(data)
	c.ExpenseCategories = &jsonData
	return nil
}

// GetSeasonalPatterns parses and returns seasonal patterns
func (c *CashflowAnalysis) GetSeasonalPatterns() ([]SeasonalPattern, error) {
	if c.SeasonalPattern == nil {
		return nil, nil
	}
	var patterns []SeasonalPattern
	if err := json.Unmarshal(*c.SeasonalPattern, &patterns); err != nil {
		return nil, err
	}
	return patterns, nil
}

// SetSeasonalPatterns sets seasonal patterns from a slice
func (c *CashflowAnalysis) SetSeasonalPatterns(patterns []SeasonalPattern) error {
	data, err := json.Marshal(patterns)
	if err != nil {
		return err
	}
	jsonData := datatypes.JSON(data)
	c.SeasonalPattern = &jsonData
	return nil
}

// GetRiskFlags parses and returns risk flags
func (c *CashflowAnalysis) GetRiskFlags() ([]RiskFlag, error) {
	if c.RiskFlags == nil {
		return nil, nil
	}
	var flags []RiskFlag
	if err := json.Unmarshal(*c.RiskFlags, &flags); err != nil {
		return nil, err
	}
	return flags, nil
}

// SetRiskFlags sets risk flags from a slice
func (c *CashflowAnalysis) SetRiskFlags(flags []RiskFlag) error {
	data, err := json.Marshal(flags)
	if err != nil {
		return err
	}
	jsonData := datatypes.JSON(data)
	c.RiskFlags = &jsonData
	return nil
}

// HasHighRiskFlags checks if there are any high severity risk flags
func (c *CashflowAnalysis) HasHighRiskFlags() (bool, error) {
	flags, err := c.GetRiskFlags()
	if err != nil {
		return false, err
	}
	for _, flag := range flags {
		if flag.Severity == "high" {
			return true, nil
		}
	}
	return false, nil
}

// --- Comparison Helpers ---
//
// CompareWith compares this analysis with another and returns the percentage change in basis points
func (c *CashflowAnalysis) CompareWith(other *CashflowAnalysis) map[string]int32 {
	changes := make(map[string]int32)

	if other.TotalIncome != 0 {
		changes["income_change_bps"] = int32((c.TotalIncome - other.TotalIncome) * 10000 / other.TotalIncome)
	}
	if other.TotalExpenses != 0 {
		changes["expenses_change_bps"] = int32((c.TotalExpenses - other.TotalExpenses) * 10000 / other.TotalExpenses)
	}
	if other.NetCashflow != 0 {
		changes["cashflow_change_bps"] = int32((c.NetCashflow - other.NetCashflow) * 10000 / other.NetCashflow)
	}
	changes["savings_rate_change_bps"] = c.SavingsRateBps - other.SavingsRateBps

	return changes
}
