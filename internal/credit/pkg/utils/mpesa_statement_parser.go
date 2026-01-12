package utils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	pdfutils "github.com/Shamba-Records-Limited/Microvault/pkg/utils"
)

// MpesaStatementParser is a utility for parsing M-Pesa statement text
type MpesaStatementParser struct {
	parser *pdfutils.PDFParser
}

// NewMpesaStatementParser creates a new instance of MpesaStatementParser
func NewMpesaStatementParser(parser *pdfutils.PDFParser) *MpesaStatementParser {
	return &MpesaStatementParser{
		parser: parser,
	}
}

// MPesaTransaction represents a parsed M-Pesa transaction
type MPesaTransaction struct {
	Date         time.Time
	Type         string
	Amount       float64
	Balance      float64
	Counterparty string
	Reference    string
	Description  string
}

// ParseMPesaStatement parses M-Pesa statement text
func (p *MpesaStatementParser) ParseMPesaStatement(text string) ([]MPesaTransaction, error) {
	var transactions []MPesaTransaction

	// M-Pesa statement patterns (adjust based on actual format)
	// Pattern 1: Date-based line detection
	datePattern := regexp.MustCompile(`(\d{2}/\d{2}/\d{4})`)

	// Pattern 2: Transaction type
	typePattern := regexp.MustCompile(`(Send Money|Receive Money|Withdraw|Buy Airtime|Buy Goods|PayBill|Till Number)`)

	// Pattern 3: Amount pattern - Ksh 1,234.56
	amountPattern := regexp.MustCompile(`Ksh\s*([\d,]+\.\d{2})`)

	// Pattern 4: Balance pattern
	balancePattern := regexp.MustCompile(`Balance:\s*Ksh\s*([\d,]+\.\d{2})`)

	// Pattern 5: Party/Counterparty
	partyPattern := regexp.MustCompile(`((?:From|To|By))\s+([A-Z\s]+(?:\d+)?)`)

	// Split into lines for processing
	lines := strings.Split(text, "\n")

	var currentTx *MPesaTransaction

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if line contains a date (potential transaction start)
		if matches := datePattern.FindStringSubmatch(line); len(matches) > 0 {
			// If we have a previous transaction, save it
			if currentTx != nil && currentTx.Date.Year() > 1 {
				transactions = append(transactions, *currentTx)
			}

			// Start new transaction
			currentTx = &MPesaTransaction{}

			// Parse date
			dateStr := matches[1]
			date, err := time.Parse("02/01/2006", dateStr)
			if err != nil {
				// Try alternative format
				date, err = time.Parse("02-01-2006", dateStr)
				if err != nil {
					continue
				}
			}
			currentTx.Date = date
		}

		if currentTx == nil {
			continue
		}

		// Extract transaction type
		if matches := typePattern.FindStringSubmatch(line); len(matches) > 0 {
			txType := matches[1]
			currentTx.Type = normalizeMPesaType(txType)
		}

		// Extract amount
		if matches := amountPattern.FindAllStringSubmatch(line, -1); len(matches) > 0 {
			// First amount is usually the transaction amount
			amountStr := strings.ReplaceAll(matches[0][1], ",", "")
			amount, err := strconv.ParseFloat(amountStr, 64)
			if err == nil {
				currentTx.Amount = amount
			}
		}

		// Extract balance
		if matches := balancePattern.FindStringSubmatch(line); len(matches) > 0 {
			balanceStr := strings.ReplaceAll(matches[1], ",", "")
			balance, err := strconv.ParseFloat(balanceStr, 64)
			if err == nil {
				currentTx.Balance = balance
			}
		}

		// Extract counterparty
		if matches := partyPattern.FindStringSubmatch(line); len(matches) > 0 {
			currentTx.Counterparty = strings.TrimSpace(matches[2])
		}

		// Store full line as description
		if currentTx.Description == "" {
			currentTx.Description = line
		} else {
			currentTx.Description += " " + line
		}
	}

	// Add the last transaction
	if currentTx != nil && currentTx.Date.Year() > 1 {
		transactions = append(transactions, *currentTx)
	}

	if len(transactions) == 0 {
		return nil, fmt.Errorf("no transactions found in statement")
	}

	return transactions, nil
}

// normalizeMPesaType converts M-Pesa transaction type to standard type
func normalizeMPesaType(mpesaType string) string {
	mpesaType = strings.ToLower(mpesaType)

	switch {
	case strings.Contains(mpesaType, "send"):
		return "send"
	case strings.Contains(mpesaType, "receive"):
		return "receive"
	case strings.Contains(mpesaType, "withdraw"):
		return "withdraw"
	case strings.Contains(mpesaType, "airtime"):
		return "airtime"
	case strings.Contains(mpesaType, "buy goods"), strings.Contains(mpesaType, "paybill"), strings.Contains(mpesaType, "till"):
		return "payment"
	default:
		return "other"
	}
}

// CategorizeTransaction attempts to categorize a transaction
func (p *MpesaStatementParser) CategorizeTransaction(txType, description, counterparty string) string {
	description = strings.ToLower(description)

	if txType == "receive" {
		// Income categorization
		if strings.Contains(description, "salary") || strings.Contains(description, "wage") {
			return "salary"
		}
		if strings.Contains(description, "business") || strings.Contains(description, "payment for") {
			return "business_income"
		}
		if strings.Contains(description, "harvest") || strings.Contains(description, "crop") || strings.Contains(description, "farm") {
			return "agricultural_income"
		}
		if strings.Contains(description, "loan") || strings.Contains(description, "borrow") {
			return "loan_received"
		}
		return "other_income"
	}

	if txType == "send" {
		// Expense categorization
		if strings.Contains(description, "rent") {
			return "rent"
		}
		if strings.Contains(description, "food") || strings.Contains(description, "grocery") {
			return "food"
		}
		if strings.Contains(description, "school") || strings.Contains(description, "education") || strings.Contains(description, "fees") {
			return "education"
		}
		if strings.Contains(description, "seed") || strings.Contains(description, "fertilizer") || strings.Contains(description, "input") {
			return "farm_inputs"
		}
		if strings.Contains(description, "loan") || strings.Contains(description, "repay") {
			return "loan_repayment"
		}
		if strings.Contains(description, "health") || strings.Contains(description, "medical") || strings.Contains(description, "hospital") {
			return "medical"
		}
		return "other_expense"
	}

	if txType == "withdraw" {
		return "withdrawal"
	}

	if txType == "airtime" {
		return "communication"
	}

	return "other"
}

// ValidateStatement checks if the parsed text looks like a valid M-Pesa statement
func (p *MpesaStatementParser) ValidateStatement(text string) error {
	// Check for M-Pesa indicators
	text = strings.ToLower(text)

	if !strings.Contains(text, "mpesa") && !strings.Contains(text, "m-pesa") {
		return fmt.Errorf("document does not appear to be an M-Pesa statement")
	}

	// Check for transaction indicators
	hasTransactions := strings.Contains(text, "transaction") ||
		strings.Contains(text, "balance") ||
		strings.Contains(text, "ksh")

	if !hasTransactions {
		return fmt.Errorf("no transaction data found in document")
	}

	return nil
}
