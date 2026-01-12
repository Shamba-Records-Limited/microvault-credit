package notifications

import (
	"context"
	"fmt"
	"time"

	"github.com/Shamba-Records-Limited/Microvault/pkg/mobile/sms"
)

// SMSNotificationService handles SMS notifications for credit operations
type SMSNotificationService struct {
	smsService   *sms.SMSService
	providerName string
	from         string
}

// NewSMSNotificationService creates a new SMS notification service
func NewSMSNotificationService(smsService *sms.SMSService, providerName string, from string) *SMSNotificationService {
	return &SMSNotificationService{
		smsService:   smsService,
		providerName: providerName,
		from:         from,
	}
}

// SendLoanRequestConfirmation sends loan request confirmation SMS
func (s *SMSNotificationService) SendLoanRequestConfirmation(ctx context.Context, phoneNumber string, loanNumber string, amountKES float64) error {
	message := fmt.Sprintf(
		"Your loan request for KES %.2f has been received (Ref: %s). You will be notified once it's approved.",
		amountKES,
		loanNumber,
	)

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendLoanApprovalNotification sends loan approval notification
func (s *SMSNotificationService) SendLoanApprovalNotification(ctx context.Context, phoneNumber string, loanNumber string, amountKES float64) error {
	message := fmt.Sprintf(
		"Congratulations! Your loan of KES %.2f has been approved (Ref: %s). It will be disbursed shortly.",
		amountKES,
		loanNumber,
	)

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendLoanDisbursementNotification sends loan disbursement notification
func (s *SMSNotificationService) SendLoanDisbursementNotification(ctx context.Context, phoneNumber string, loanNumber string, amountKES float64) error {
	message := fmt.Sprintf(
		"Your loan of KES %.2f has been disbursed (Ref: %s). The funds are now available in your account.",
		amountKES,
		loanNumber,
	)

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendRepaymentConfirmation sends repayment confirmation
func (s *SMSNotificationService) SendRepaymentConfirmation(ctx context.Context, phoneNumber string, loanNumber string, amountPaidKES float64, remainingBalanceKES float64) error {
	message := fmt.Sprintf(
		"Payment of KES %.2f received for loan %s. Remaining balance: KES %.2f. Thank you!",
		amountPaidKES,
		loanNumber,
		remainingBalanceKES,
	)

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendRepaymentReminder sends repayment reminder
func (s *SMSNotificationService) SendRepaymentReminder(ctx context.Context, phoneNumber string, loanNumber string, amountDueKES float64, dueDate time.Time) error {
	daysUntilDue := int(time.Until(dueDate).Hours() / 24)

	var message string
	if daysUntilDue <= 0 {
		message = fmt.Sprintf(
			"URGENT: Your loan payment of KES %.2f is overdue (Ref: %s). Please pay immediately to avoid penalties.",
			amountDueKES,
			loanNumber,
		)
	} else if daysUntilDue <= 3 {
		message = fmt.Sprintf(
			"Reminder: Your loan payment of KES %.2f is due in %d days (Ref: %s). Dial *384*1234# to pay.",
			amountDueKES,
			daysUntilDue,
			loanNumber,
		)
	} else {
		message = fmt.Sprintf(
			"Reminder: Your loan payment of KES %.2f is due on %s (Ref: %s).",
			amountDueKES,
			dueDate.Format("2006-01-02"),
			loanNumber,
		)
	}

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendLoanDefaultNotification sends loan default notification
func (s *SMSNotificationService) SendLoanDefaultNotification(ctx context.Context, phoneNumber string, loanNumber string) error {
	message := fmt.Sprintf(
		"Your loan (Ref: %s) has been marked as defaulted. This will affect your credit score. Please contact support.",
		loanNumber,
	)

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendWelcomeMessage sends welcome message to new users
func (s *SMSNotificationService) SendWelcomeMessage(ctx context.Context, phoneNumber string, name string) error {
	message := fmt.Sprintf(
		"Welcome to MicroVault, %s! Dial *384*1234# to access your account and request loans. Need help? Visit https://microvault.com/support",
		name,
	)

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendKYCVerificationNotification sends KYC verification notification
func (s *SMSNotificationService) SendKYCVerificationNotification(ctx context.Context, phoneNumber string, status string) error {
	var message string
	switch status {
	case "verified":
		message = "Your identity has been verified! You can now request higher loan amounts. Dial *384*1234# to get started."
	case "rejected":
		message = "Your identity verification was not successful. Please contact support for assistance."
	default:
		message = "Your identity is being verified. You will be notified once the process is complete."
	}

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendAccountSuspensionNotification sends account suspension notification
func (s *SMSNotificationService) SendAccountSuspensionNotification(ctx context.Context, phoneNumber string, reason string) error {
	message := fmt.Sprintf(
		"Your MicroVault account has been suspended. Reason: %s. Please contact support.",
		reason,
	)

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendCreditScoreUpdate sends credit score update notification
func (s *SMSNotificationService) SendCreditScoreUpdate(ctx context.Context, phoneNumber string, score int, maxLoanAmount int64) error {
	message := fmt.Sprintf(
		"Your credit score has been updated to %d. Maximum loan amount: KES %.2f. Keep up the good work!",
		score,
		float64(maxLoanAmount)/100, // Assuming amount is in cents
	)

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendSecurityAlert sends security alert
func (s *SMSNotificationService) SendSecurityAlert(ctx context.Context, phoneNumber string, alert string) error {
	message := fmt.Sprintf(
		"Security Alert: %s. If this wasn't you, please contact support immediately.",
		alert,
	)

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendPromotionalMessage sends promotional message
func (s *SMSNotificationService) SendPromotionalMessage(ctx context.Context, phoneNumber string, promotion string) error {
	message := fmt.Sprintf(
		"MicroVault: %s Dial *384*1234# to take advantage of this offer!",
		promotion,
	)

	return s.sendSMS(ctx, phoneNumber, message)
}

// SendBulkNotification sends notification to multiple recipients
func (s *SMSNotificationService) SendBulkNotification(ctx context.Context, phoneNumbers []string, message string) error {
	provider, err := s.smsService.GetProvider(s.providerName)
	if err != nil {
		return fmt.Errorf("failed to get SMS provider: %w", err)
	}

	_, err = provider.SendBulkSMS(ctx, phoneNumbers, message, s.from)
	if err != nil {
		return fmt.Errorf("failed to send bulk SMS: %w", err)
	}

	return nil
}

// sendSMS is a helper to send a single SMS
func (s *SMSNotificationService) sendSMS(ctx context.Context, phoneNumber, message string) error {
	provider, err := s.smsService.GetProvider(s.providerName)
	if err != nil {
		return fmt.Errorf("failed to get SMS provider: %w", err)
	}

	_, err = provider.SendSingleSMS(ctx, phoneNumber, message, s.from)
	if err != nil {
		return fmt.Errorf("failed to send SMS to %s: %w", phoneNumber, err)
	}

	return nil
}

// FormatPhoneNumber formats a phone number for SMS providers
func FormatPhoneNumber(phoneNumber, countryCode string) string {
	// Remove any spaces, dashes, or parentheses
	cleaned := ""
	for _, r := range phoneNumber {
		if r >= '0' && r <= '9' {
			cleaned += string(r)
		}
	}

	// Add country code if missing
	if !startsWith(cleaned, "+") && !startsWith(cleaned, "00") {
		if countryCode == "" {
			countryCode = "+254" // Default to Kenya
		}
		cleaned = countryCode + cleaned
	}

	return cleaned
}

// startsWith checks if string starts with prefix
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
