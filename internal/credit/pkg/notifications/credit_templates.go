package notifications

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	mvnotifications "github.com/Shamba-Records-Limited/microvault/pkg/notifications"
)

// supportURL is where credit messages point users who need help. Static per
// deployment, unlike the USSD service code.
const supportURL = "https://microvault.com/support"

// creditTemplates returns the Shamba Records credit copy keyed by ISO language
// code, with dialString captured into every message that quotes it.
func creditTemplates(dialString string) map[string]*CreditTemplates {
	return map[string]*CreditTemplates{
		"en": {
			ScoreUpdate: func(n CreditNotification) string {
				return fmt.Sprintf("Your %s credit score has been updated to %d. "+
					"Maximum loan amount: %s %.2f. Keep up the good work!",
					brandName, n.Score, n.Currency, n.MaxLoanAmount)
			},
			AccountSuspended: func(n CreditNotification) string {
				return fmt.Sprintf("Your %s account has been suspended. Reason: %s. Please contact support.",
					brandName, n.Reason)
			},
			Welcome: func(n CreditNotification) string {
				return fmt.Sprintf("Welcome to %s, %s! Dial %s to access your account and request loans. "+
					"Need help? Visit %s", brandName, n.FullName, dialString, supportURL)
			},
			KYCVerified: func(CreditNotification) string {
				return fmt.Sprintf("Your identity has been verified! You can now request higher loan "+
					"amounts. Dial %s to get started.", dialString)
			},
			KYCRejected: func(CreditNotification) string {
				return "Your identity verification was not successful. Please contact support for assistance."
			},
			KYCPending: func(CreditNotification) string {
				return "Your identity is being verified. You will be notified once the process is complete."
			},
			SecurityAlert: func(n CreditNotification) string {
				return fmt.Sprintf("Security Alert: %s. If this wasn't you, please contact support immediately.",
					n.Alert)
			},
			PromotionalOffer: func(n CreditNotification) string {
				return fmt.Sprintf("%s: %s Dial %s to take advantage of this offer!",
					brandName, n.Promotion, dialString)
			},
		},
		"sw": {
			ScoreUpdate: func(n CreditNotification) string {
				return fmt.Sprintf("Alama yako ya mkopo ya %s imesasishwa hadi %d. "+
					"Kiwango cha juu cha mkopo: %s %.2f. Endelea na kazi nzuri!",
					brandName, n.Score, n.Currency, n.MaxLoanAmount)
			},
			AccountSuspended: func(n CreditNotification) string {
				return fmt.Sprintf("Akaunti yako ya %s imesimamishwa. Sababu: %s. Tafadhali wasiliana na msaada.",
					brandName, n.Reason)
			},
			Welcome: func(n CreditNotification) string {
				return fmt.Sprintf("Karibu %s, %s! Piga %s kufikia akaunti yako na kuomba mikopo. "+
					"Unahitaji msaada? Tembelea %s", brandName, n.FullName, dialString, supportURL)
			},
			KYCVerified: func(CreditNotification) string {
				return fmt.Sprintf("Utambulisho wako umethibitishwa! Sasa unaweza kuomba mikopo mikubwa "+
					"zaidi. Piga %s kuanza.", dialString)
			},
			KYCRejected: func(CreditNotification) string {
				return "Uthibitishaji wa utambulisho wako haukufanikiwa. Tafadhali wasiliana na msaada."
			},
			KYCPending: func(CreditNotification) string {
				return "Utambulisho wako unathibitishwa. Utaarifiwa mchakato utakapokamilika."
			},
			SecurityAlert: func(n CreditNotification) string {
				return fmt.Sprintf("Tahadhari ya Usalama: %s. Kama hukuwa wewe, wasiliana na msaada mara moja.",
					n.Alert)
			},
			PromotionalOffer: func(n CreditNotification) string {
				return fmt.Sprintf("%s: %s Piga %s kunufaika na ofa hii!",
					brandName, n.Promotion, dialString)
			},
		},
		"fr": {
			ScoreUpdate: func(n CreditNotification) string {
				return fmt.Sprintf("Votre score de crédit %s a été mis à jour à %d. "+
					"Montant maximum du pret: %s %.2f. Continuez ainsi!",
					brandName, n.Score, n.Currency, n.MaxLoanAmount)
			},
			AccountSuspended: func(n CreditNotification) string {
				return fmt.Sprintf("Votre compte %s a été suspendu. Raison: %s. Veuillez contacter le support.",
					brandName, n.Reason)
			},
			Welcome: func(n CreditNotification) string {
				return fmt.Sprintf("Bienvenue à %s, %s! Composez %s pour accéder à votre compte et demander "+
					"des prets. Besoin d'aide? Visitez %s", brandName, n.FullName, dialString, supportURL)
			},
			KYCVerified: func(CreditNotification) string {
				return fmt.Sprintf("Votre identité a été vérifiée! Vous pouvez maintenant demander des "+
					"montants plus élevés. Composez %s pour commencer.", dialString)
			},
			KYCRejected: func(CreditNotification) string {
				return "La vérification de votre identité a échoué. Veuillez contacter le support."
			},
			KYCPending: func(CreditNotification) string {
				return "Votre identité est en cours de vérification. Vous serez notifié une fois le processus terminé."
			},
			SecurityAlert: func(n CreditNotification) string {
				return fmt.Sprintf("Alerte de Sécurité: %s. Si ce n'était pas vous, contactez le support immédiatement.",
					n.Alert)
			},
			PromotionalOffer: func(n CreditNotification) string {
				return fmt.Sprintf("%s: %s Composez %s pour profiter de cette offre!",
					brandName, n.Promotion, dialString)
			},
		},
	}
}

// sentinelCreditNotification is a fully-populated notification used to render
// every template at construction, mirroring the platform's own sentinels.
func sentinelCreditNotification() CreditNotification {
	return CreditNotification{
		PhoneNumber:   "254711000111",
		FullName:      "Alice Wanjiku Kamau",
		Score:         720,
		MaxLoanAmount: 15000.00,
		Currency:      "KES",
		Reason:        "repeated late repayment",
		Alert:         "a new device signed in to your account",
		Promotion:     "Zero interest on your next loan this week.",
	}
}

// validateCreditTemplates renders one language's set against the sentinel and
// rejects fields that are unset, render empty, or fall outside GSM 03.38.
func validateCreditTemplates(lang string, set *CreditTemplates) error {
	n := sentinelCreditNotification()
	v := reflect.ValueOf(set).Elem()
	t := v.Type()

	var problems []string
	for i := range v.NumField() {
		name := t.Field(i).Name
		field := v.Field(i)
		if field.IsNil() {
			problems = append(problems, fmt.Sprintf("%s is not set", name))
			continue
		}
		msg := field.Interface().(CreditMessage)(n)
		if msg == "" {
			problems = append(problems, fmt.Sprintf("%s renders an empty message", name))
			continue
		}
		if _, bad, ok := mvnotifications.GSM7Len(msg); !ok {
			problems = append(problems, fmt.Sprintf("%s contains %q, outside GSM 03.38", name, bad))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("%s credit templates: %s", lang, strings.Join(problems, "; "))
}
