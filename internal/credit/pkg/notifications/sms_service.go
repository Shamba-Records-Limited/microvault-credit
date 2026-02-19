package notifications

import (
	"context"
	"fmt"

	mvnotifications "github.com/Shamba-Records-Limited/microvault/pkg/notifications"
)

// CreditNotificationService handles credit-specific notifications that are not
// part of the core loan lifecycle (those live in pkg/notifications in microvault).
type CreditNotificationService struct {
	notifier mvnotifications.Notifier
}

// NewCreditNotificationService creates a new CreditNotificationService.
func NewCreditNotificationService(notifier mvnotifications.Notifier) *CreditNotificationService {
	return &CreditNotificationService{notifier: notifier}
}

// SendCreditScoreUpdate sends credit score update notification
func (s *CreditNotificationService) SendCreditScoreUpdate(ctx context.Context, phoneNumber string, score int, maxLoanAmount int64) error {
	message := fmt.Sprintf(
		"Your credit score has been updated to %d. Maximum loan amount: KES %.2f. Keep up the good work!",
		score,
		float64(maxLoanAmount)/100,
	)
	return s.notifier.Send(ctx, phoneNumber, message)
}

// SendAccountSuspensionNotification sends account suspension notification
func (s *CreditNotificationService) SendAccountSuspensionNotification(ctx context.Context, phoneNumber string, reason string) error {
	message := fmt.Sprintf(
		"Your microvault account has been suspended. Reason: %s. Please contact support.",
		reason,
	)
	return s.notifier.Send(ctx, phoneNumber, message)
}

// SendWelcomeMessage sends welcome message to new users
func (s *CreditNotificationService) SendWelcomeMessage(ctx context.Context, phoneNumber string, name string) error {
	message := fmt.Sprintf(
		"Welcome to microvault, %s! Dial *384*1234# to access your account and request loans. Need help? Visit https://microvault.com/support",
		name,
	)
	return s.notifier.Send(ctx, phoneNumber, message)
}

// SendKYCVerificationNotification sends KYC verification notification
func (s *CreditNotificationService) SendKYCVerificationNotification(ctx context.Context, phoneNumber string, status string) error {
	var message string
	switch status {
	case "verified":
		message = "Your identity has been verified! You can now request higher loan amounts. Dial *384*1234# to get started."
	case "rejected":
		message = "Your identity verification was not successful. Please contact support for assistance."
	default:
		message = "Your identity is being verified. You will be notified once the process is complete."
	}
	return s.notifier.Send(ctx, phoneNumber, message)
}

// SendSecurityAlert sends security alert
func (s *CreditNotificationService) SendSecurityAlert(ctx context.Context, phoneNumber string, alert string) error {
	message := fmt.Sprintf(
		"Security Alert: %s. If this wasn't you, please contact support immediately.",
		alert,
	)
	return s.notifier.Send(ctx, phoneNumber, message)
}

// SendPromotionalMessage sends promotional message
func (s *CreditNotificationService) SendPromotionalMessage(ctx context.Context, phoneNumber string, promotion string) error {
	message := fmt.Sprintf(
		"microvault: %s Dial *384*1234# to take advantage of this offer!",
		promotion,
	)
	return s.notifier.Send(ctx, phoneNumber, message)
}

// SendBulkNotification sends notification to multiple recipients.
// Note: this sends individual messages via the Notifier interface. For true
// bulk SMS, consumers should use the SMS provider directly.
func (s *CreditNotificationService) SendBulkNotification(ctx context.Context, phoneNumbers []string, message string) error {
	for _, phone := range phoneNumbers {
		if err := s.notifier.Send(ctx, phone, message); err != nil {
			return fmt.Errorf("failed to send to %s: %w", phone, err)
		}
	}
	return nil
}
