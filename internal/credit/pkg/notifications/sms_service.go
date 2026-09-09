package notifications

import (
	"context"
	"fmt"

	mvnotifications "github.com/Shamba-Records-Limited/microvault/pkg/notifications"
)

// CreditNotification carries the data for credit-specific messages — the ones
// outside the loan and account lifecycles the platform owns.
type CreditNotification struct {
	PhoneNumber string
	FullName    string
	// Score and MaxLoanAmount back the credit score update. MaxLoanAmount is
	// display-ready in Currency units, not minor units.
	Score         int
	MaxLoanAmount float64
	Currency      string
	// Reason explains a suspension; Alert describes a security event;
	// Promotion carries marketing copy supplied by the caller.
	Reason    string
	Alert     string
	Promotion string
	// Language pins the SMS language (en/sw/fr). Empty resolves through the
	// service's LanguageResolver, then falls back to English.
	Language string
}

// CreditMessage renders one credit notification.
type CreditMessage func(n CreditNotification) string

// CreditTemplates holds one renderer per credit-specific event.
type CreditTemplates struct {
	ScoreUpdate      CreditMessage
	AccountSuspended CreditMessage
	Welcome          CreditMessage
	KYCVerified      CreditMessage
	KYCRejected      CreditMessage
	KYCPending       CreditMessage
	SecurityAlert    CreditMessage
	PromotionalOffer CreditMessage
}

// CreditNotificationService sends credit-specific notifications. Loan lifecycle
// and account/PIN messages are handled by the platform notifiers in
// microvault/pkg/notifications.
type CreditNotificationService struct {
	notifier    mvnotifications.Notifier
	templates   map[string]*CreditTemplates
	resolveLang mvnotifications.LanguageResolver
}

// NewCreditNotificationService creates a service over the Shamba Records credit
// copy. dialString is the dialled USSD string for this deployment; resolve may
// be nil, in which case notifications without a pinned language render in
// English.
func NewCreditNotificationService(
	notifier mvnotifications.Notifier,
	dialString string,
	resolve mvnotifications.LanguageResolver,
) (*CreditNotificationService, error) {
	set := creditTemplates(dialString)
	for lang, tmpl := range set {
		if err := validateCreditTemplates(lang, tmpl); err != nil {
			return nil, err
		}
	}
	return &CreditNotificationService{notifier: notifier, templates: set, resolveLang: resolve}, nil
}

// tmpl selects the template set for a notification: its pinned Language, else
// the resolved recipient preference, else English.
func (s *CreditNotificationService) tmpl(ctx context.Context, n CreditNotification) *CreditTemplates {
	lang := n.Language
	if lang == "" && s.resolveLang != nil {
		lang = s.resolveLang(ctx, n.PhoneNumber)
	}
	if t, ok := s.templates[lang]; ok {
		return t
	}
	return s.templates["en"]
}

func (s *CreditNotificationService) send(ctx context.Context, n CreditNotification, event string, msg CreditMessage) error {
	if err := s.notifier.Send(ctx, n.PhoneNumber, msg(n)); err != nil {
		return fmt.Errorf("notify %s: %w", event, err)
	}
	return nil
}

// SendCreditScoreUpdate tells a user their score and new borrowing ceiling.
func (s *CreditNotificationService) SendCreditScoreUpdate(ctx context.Context, n CreditNotification) error {
	return s.send(ctx, n, "credit score update", s.tmpl(ctx, n).ScoreUpdate)
}

// SendAccountSuspension tells a user their account has been suspended.
func (s *CreditNotificationService) SendAccountSuspension(ctx context.Context, n CreditNotification) error {
	return s.send(ctx, n, "account suspension", s.tmpl(ctx, n).AccountSuspended)
}

// SendWelcomeMessage greets a newly onboarded user.
func (s *CreditNotificationService) SendWelcomeMessage(ctx context.Context, n CreditNotification) error {
	return s.send(ctx, n, "welcome", s.tmpl(ctx, n).Welcome)
}

// SendKYCVerified confirms a successful identity check.
func (s *CreditNotificationService) SendKYCVerified(ctx context.Context, n CreditNotification) error {
	return s.send(ctx, n, "KYC verified", s.tmpl(ctx, n).KYCVerified)
}

// SendKYCRejected reports a failed identity check.
func (s *CreditNotificationService) SendKYCRejected(ctx context.Context, n CreditNotification) error {
	return s.send(ctx, n, "KYC rejected", s.tmpl(ctx, n).KYCRejected)
}

// SendKYCPending acknowledges an identity check still in progress.
func (s *CreditNotificationService) SendKYCPending(ctx context.Context, n CreditNotification) error {
	return s.send(ctx, n, "KYC pending", s.tmpl(ctx, n).KYCPending)
}

// SendSecurityAlert warns a user about activity on their account.
func (s *CreditNotificationService) SendSecurityAlert(ctx context.Context, n CreditNotification) error {
	return s.send(ctx, n, "security alert", s.tmpl(ctx, n).SecurityAlert)
}

// SendPromotionalMessage sends marketing copy supplied by the caller.
func (s *CreditNotificationService) SendPromotionalMessage(ctx context.Context, n CreditNotification) error {
	return s.send(ctx, n, "promotional offer", s.tmpl(ctx, n).PromotionalOffer)
}

// SendBulkNotification sends one already-rendered message to many recipients.
// It loops over the Notifier rather than using a provider bulk endpoint, so it
// is unsuitable for large lists; callers needing true bulk SMS should reach for
// the SMS provider directly.
func (s *CreditNotificationService) SendBulkNotification(ctx context.Context, phoneNumbers []string, message string) error {
	for _, phone := range phoneNumbers {
		if err := s.notifier.Send(ctx, phone, message); err != nil {
			return fmt.Errorf("send bulk notification: %w", err)
		}
	}
	return nil
}
