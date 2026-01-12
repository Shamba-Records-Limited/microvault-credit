package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
)

// FraudDetectionService detects fraudulent activity in credit scoring
type FraudDetectionService struct {
	// Dependencies would be injected
}

// NewFraudDetectionService creates a new fraud detection service
func NewFraudDetectionService() *FraudDetectionService {
	return &FraudDetectionService{}
}

// FraudAlert represents a fraud detection alert
type FraudAlert struct {
	UserID      string
	AlertType   string
	Severity    string // CRITICAL, HIGH, MEDIUM, LOW
	Description string
	Evidence    map[string]interface{}
	DetectedAt  time.Time
	RiskScore   int // 0-100
}

// =====================================================
// Transaction Fraud Detection
// =====================================================

// DetectCircularTransactions identifies suspicious send/receive patterns
func (s *FraudDetectionService) DetectCircularTransactions(ctx context.Context, transactions []models.TransactionExtracted) []FraudAlert {
	alerts := []FraudAlert{}

	// Build transaction graph
	sent := make(map[string]int64)
	received := make(map[string]int64)

	for _, tx := range transactions {
		if tx.Counterparty == nil {
			continue
		}

		counterparty := *tx.Counterparty
		switch tx.TransactionType {
		case "send":
			sent[counterparty] += tx.Amount
		case "receive":
			received[counterparty] += tx.Amount
		}
	}

	// Detect circular patterns
	for counterparty, sentAmount := range sent {
		receivedAmount := received[counterparty]

		if sentAmount > 0 && receivedAmount > 0 {
			ratio := float64(receivedAmount) / float64(sentAmount)

			// If amounts are nearly equal (±20%), flag as suspicious
			if ratio > 0.8 && ratio < 1.2 {
				alerts = append(alerts, FraudAlert{
					AlertType:   "circular_transactions",
					Severity:    "HIGH",
					Description: fmt.Sprintf("Circular money movement with %s", counterparty),
					Evidence: map[string]interface{}{
						"counterparty":    counterparty,
						"sent_amount":     sentAmount,
						"received_amount": receivedAmount,
						"ratio":           ratio,
					},
					DetectedAt: time.Now(),
					RiskScore:  75,
				})
			}
		}
	}

	return alerts
}

// DetectDuplicateTransactions checks for duplicate transaction entries
func (s *FraudDetectionService) DetectDuplicateTransactions(ctx context.Context, transactions []models.TransactionExtracted) []FraudAlert {
	alerts := []FraudAlert{}

	seen := make(map[string]int)

	for _, tx := range transactions {
		// Create hash of transaction
		hash := s.hashTransaction(&tx)
		seen[hash]++

		if seen[hash] > 1 {
			alerts = append(alerts, FraudAlert{
				AlertType:   "duplicate_transaction",
				Severity:    "CRITICAL",
				Description: fmt.Sprintf("Transaction appears %d times", seen[hash]),
				Evidence: map[string]interface{}{
					"date":         tx.TransactionDate,
					"amount":       tx.Amount,
					"counterparty": tx.Counterparty,
					"count":        seen[hash],
				},
				DetectedAt: time.Now(),
				RiskScore:  90,
			})
		}
	}

	return alerts
}

// DetectUnrealisticTransactions identifies impossible transaction patterns
func (s *FraudDetectionService) DetectUnrealisticTransactions(ctx context.Context, transactions []models.TransactionExtracted) []FraudAlert {
	alerts := []FraudAlert{}

	// Check for unrealistic amounts
	for _, tx := range transactions {
		// Single transaction too large (>10M KES = 100K USD in smallest unit: 10,000,000,00 cents)
		if tx.Amount > 1000000000 {
			alerts = append(alerts, FraudAlert{
				AlertType:   "unrealistic_amount",
				Severity:    "HIGH",
				Description: fmt.Sprintf("Unusually large transaction: %d (smallest unit)", tx.Amount),
				Evidence: map[string]interface{}{
					"amount": tx.Amount,
					"date":   tx.TransactionDate,
				},
				DetectedAt: time.Now(),
				RiskScore:  70,
			})
		}

		// Transaction in future
		if tx.TransactionDate.After(time.Now()) {
			alerts = append(alerts, FraudAlert{
				AlertType:   "future_transaction",
				Severity:    "CRITICAL",
				Description: "Transaction date is in the future",
				Evidence: map[string]interface{}{
					"date": tx.TransactionDate,
				},
				DetectedAt: time.Now(),
				RiskScore:  95,
			})
		}
	}

	// Check transaction velocity
	dailyCount := make(map[string]int)
	for _, tx := range transactions {
		day := tx.TransactionDate.Format("2006-01-02")
		dailyCount[day]++
	}

	for day, count := range dailyCount {
		if count > 50 {
			alerts = append(alerts, FraudAlert{
				AlertType:   "high_velocity",
				Severity:    "MEDIUM",
				Description: fmt.Sprintf("Unusually high transaction count: %d on %s", count, day),
				Evidence: map[string]interface{}{
					"date":  day,
					"count": count,
				},
				DetectedAt: time.Now(),
				RiskScore:  50,
			})
		}
	}

	// Check for all round numbers (suspicious)
	roundNumberCount := 0
	for _, tx := range transactions {
		if int(tx.Amount)%100 == 0 {
			roundNumberCount++
		}
	}

	roundNumberPercentage := float64(roundNumberCount) / float64(len(transactions)) * 100
	if roundNumberPercentage > 90 && len(transactions) > 10 {
		alerts = append(alerts, FraudAlert{
			AlertType:   "all_round_numbers",
			Severity:    "MEDIUM",
			Description: fmt.Sprintf("%.1f%% of transactions are round numbers", roundNumberPercentage),
			Evidence: map[string]interface{}{
				"percentage":  roundNumberPercentage,
				"total_count": len(transactions),
				"round_count": roundNumberCount,
			},
			DetectedAt: time.Now(),
			RiskScore:  60,
		})
	}

	return alerts
}

// =====================================================
// Credit Score Fraud Detection
// =====================================================

// DetectRapidCreditScoreChanges identifies suspicious score manipulation
func (s *FraudDetectionService) DetectRapidCreditScoreChanges(ctx context.Context, userID string, scores []models.CreditScore) []FraudAlert {
	alerts := []FraudAlert{}

	if len(scores) < 2 {
		return alerts
	}

	// Check for rapid changes
	for i := 1; i < len(scores); i++ {
		prev := scores[i-1]
		curr := scores[i]

		timeDiff := curr.CalculatedAt.Sub(prev.CalculatedAt)
		scoreDiff := curr.Score - prev.Score

		// Score changed by >200 points in <24 hours
		if timeDiff < 24*time.Hour && abs(scoreDiff) > 200 {
			alerts = append(alerts, FraudAlert{
				UserID:      userID,
				AlertType:   "rapid_score_change",
				Severity:    "HIGH",
				Description: fmt.Sprintf("Credit score changed by %d points in %.1f hours", scoreDiff, timeDiff.Hours()),
				Evidence: map[string]interface{}{
					"old_score": prev.Score,
					"new_score": curr.Score,
					"time_diff": timeDiff.Hours(),
				},
				DetectedAt: time.Now(),
				RiskScore:  80,
			})
		}

		// Multiple recalculations in short period
		if timeDiff < 1*time.Hour {
			alerts = append(alerts, FraudAlert{
				UserID:      userID,
				AlertType:   "frequent_recalculation",
				Severity:    "MEDIUM",
				Description: fmt.Sprintf("Credit score recalculated %.1f minutes apart", timeDiff.Minutes()),
				Evidence: map[string]interface{}{
					"time_diff_minutes": timeDiff.Minutes(),
				},
				DetectedAt: time.Now(),
				RiskScore:  50,
			})
		}
	}

	return alerts
}

// =====================================================
// Admin Activity Fraud Detection
// =====================================================

// DetectSuspiciousAdminActivity identifies unusual admin behavior
func (s *FraudDetectionService) DetectSuspiciousAdminActivity(ctx context.Context, adminID string, auditLogs []models.CreditConfigAuditLog) []FraudAlert {
	alerts := []FraudAlert{}

	if len(auditLogs) == 0 {
		return alerts
	}

	// Check for rapid configuration changes
	if len(auditLogs) > 10 {
		firstChange := auditLogs[0].CreatedAt
		lastChange := auditLogs[len(auditLogs)-1].CreatedAt
		duration := lastChange.Sub(firstChange)

		if duration < 1*time.Hour {
			alerts = append(alerts, FraudAlert{
				UserID:      adminID,
				AlertType:   "excessive_config_changes",
				Severity:    "HIGH",
				Description: fmt.Sprintf("%d configuration changes in %.1f minutes", len(auditLogs), duration.Minutes()),
				Evidence: map[string]interface{}{
					"change_count": len(auditLogs),
					"duration":     duration.Minutes(),
				},
				DetectedAt: time.Now(),
				RiskScore:  75,
			})
		}
	}

	// Check for large factor weight changes
	for _, log := range auditLogs {
		if log.ConfigTable == "credit_scoring_factors" && log.Action == "update" {
			if log.OldValues != nil && log.NewValues != nil {
				oldWeight, okOld := log.OldValues["weight"].(float64)
				newWeight, okNew := log.NewValues["weight"].(float64)

				if okOld && okNew {
					diff := newWeight - oldWeight

					if abs(int(diff*100)) > 20 { // More than 20% change
						alerts = append(alerts, FraudAlert{
							UserID:      adminID,
							AlertType:   "large_weight_change",
							Severity:    "HIGH",
							Description: fmt.Sprintf("Factor weight changed by %.1f%%", diff*100),
							Evidence: map[string]interface{}{
								"old_weight": oldWeight,
								"new_weight": newWeight,
								"config_id":  log.ConfigID,
							},
							DetectedAt: time.Now(),
							RiskScore:  70,
						})
					}
				}
			}
		}
	}

	// Check for off-hours activity
	for _, log := range auditLogs {
		hour := log.CreatedAt.Hour()
		if hour < 6 || hour > 22 {
			alerts = append(alerts, FraudAlert{
				UserID:      adminID,
				AlertType:   "off_hours_activity",
				Severity:    "MEDIUM",
				Description: fmt.Sprintf("Configuration change at %s", log.CreatedAt.Format("15:04")),
				Evidence: map[string]interface{}{
					"hour":        hour,
					"action":      log.Action,
					"config_type": log.ConfigTable,
				},
				DetectedAt: time.Now(),
				RiskScore:  40,
			})
		}
	}

	return alerts
}

// =====================================================
// Loan Application Fraud Detection
// =====================================================

// DetectMultipleLoanAttempts identifies users attempting to bypass credit limits
func (s *FraudDetectionService) DetectMultipleLoanAttempts(ctx context.Context, userID string, loans []models.Loan) []FraudAlert {
	alerts := []FraudAlert{}

	// Get recent loan applications (last 24 hours)
	recentLoans := []models.Loan{}
	for _, loan := range loans {
		if time.Since(loan.CreatedAt) < 24*time.Hour {
			recentLoans = append(recentLoans, loan)
		}
	}

	// Too many applications in short period
	if len(recentLoans) > 3 {
		alerts = append(alerts, FraudAlert{
			UserID:      userID,
			AlertType:   "excessive_loan_applications",
			Severity:    "HIGH",
			Description: fmt.Sprintf("%d loan applications in last 24 hours", len(recentLoans)),
			Evidence: map[string]interface{}{
				"application_count": len(recentLoans),
				"period_hours":      24,
			},
			DetectedAt: time.Now(),
			RiskScore:  65,
		})
	}

	// Multiple simultaneous pending applications
	pendingCount := 0
	for _, loan := range loans {
		if loan.Status == models.LoanStatusPending {
			pendingCount++
		}
	}

	if pendingCount > 2 {
		alerts = append(alerts, FraudAlert{
			UserID:      userID,
			AlertType:   "multiple_pending_loans",
			Severity:    "HIGH",
			Description: fmt.Sprintf("%d pending loan applications", pendingCount),
			Evidence: map[string]interface{}{
				"pending_count": pendingCount,
			},
			DetectedAt: time.Now(),
			RiskScore:  70,
		})
	}

	return alerts
}

// =====================================================
// Statement Validation
// =====================================================

// ValidatePDFMetadata checks if PDF appears to be genuine M-Pesa statement
func (s *FraudDetectionService) ValidatePDFMetadata(pdfData []byte) []FraudAlert {
	alerts := []FraudAlert{}

	// Extract PDF metadata
	metadata := extractPDFMetadata(pdfData)

	// Check creator
	if metadata.Creator != "M-Pesa Statement Generator" &&
		metadata.Creator != "Safaricom" &&
		metadata.Creator != "M-PESA" {
		alerts = append(alerts, FraudAlert{
			AlertType:   "invalid_pdf_creator",
			Severity:    "CRITICAL",
			Description: fmt.Sprintf("PDF creator is '%s', not M-Pesa system", metadata.Creator),
			Evidence: map[string]interface{}{
				"creator": metadata.Creator,
			},
			DetectedAt: time.Now(),
			RiskScore:  95,
		})
	}

	// Check if modified after creation
	if metadata.ModDate.After(metadata.CreationDate.Add(24 * time.Hour)) {
		alerts = append(alerts, FraudAlert{
			AlertType:   "modified_pdf",
			Severity:    "CRITICAL",
			Description: "PDF was modified after creation",
			Evidence: map[string]interface{}{
				"creation_date":     metadata.CreationDate,
				"modification_date": metadata.ModDate,
			},
			DetectedAt: time.Now(),
			RiskScore:  90,
		})
	}

	// Check if statement is too old
	if time.Since(metadata.CreationDate) > 365*24*time.Hour {
		alerts = append(alerts, FraudAlert{
			AlertType:   "outdated_statement",
			Severity:    "MEDIUM",
			Description: fmt.Sprintf("Statement is %.0f days old", time.Since(metadata.CreationDate).Hours()/24),
			Evidence: map[string]interface{}{
				"creation_date": metadata.CreationDate,
				"age_days":      int(time.Since(metadata.CreationDate).Hours() / 24),
			},
			DetectedAt: time.Now(),
			RiskScore:  40,
		})
	}

	return alerts
}

// =====================================================
// Helper Functions
// =====================================================

func (s *FraudDetectionService) hashTransaction(tx *models.TransactionExtracted) string {
	data := fmt.Sprintf("%s:%s:%d:%v",
		tx.UserID,
		tx.TransactionDate.Format("2006-01-02 15:04:05"),
		tx.Amount,
		tx.Counterparty,
	)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// PDFMetadata represents extracted PDF metadata
type PDFMetadata struct {
	Creator      string
	CreationDate time.Time
	ModDate      time.Time
	Producer     string
}

// extractPDFMetadata extracts metadata from PDF (placeholder)
func extractPDFMetadata(pdfData []byte) PDFMetadata {
	// TODO: Implement using PDF library
	// This would extract Creator, CreationDate, ModDate, Producer fields
	return PDFMetadata{
		Creator:      "Unknown",
		CreationDate: time.Now(),
		ModDate:      time.Now(),
		Producer:     "Unknown",
	}
}

// CalculateOverallRiskScore aggregates all fraud alerts into a single risk score
func (s *FraudDetectionService) CalculateOverallRiskScore(alerts []FraudAlert) int {
	if len(alerts) == 0 {
		return 0
	}

	totalRisk := 0
	for _, alert := range alerts {
		totalRisk += alert.RiskScore
	}

	// Average risk score
	avgRisk := totalRisk / len(alerts)

	// Boost score if multiple high-severity alerts
	criticalCount := 0
	highCount := 0
	for _, alert := range alerts {
		switch alert.Severity {
		case "CRITICAL":
			criticalCount++
		case "HIGH":
			highCount++
		}
	}

	// Add penalty for multiple severe alerts
	penalty := criticalCount*15 + highCount*10

	finalRisk := avgRisk + penalty
	min := func(x, y int) int {
		if x < y {
			return x
		}
		return y
	}
	finalRisk = min(finalRisk, 100)

	return finalRisk
}
