package notifications

import (
	"fmt"

	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	mvnotifications "github.com/Shamba-Records-Limited/microvault/pkg/notifications"
)

// LoanOverrides returns the loan messages that differ from the platform
// defaults, keyed by ISO language code. Only the messages that quote the USSD
// service code are overridden; the rest describe platform mechanics and are
// left to microvault. RepaymentExpired is here because telling a borrower to
// start over is only actionable if it says what to dial.
func LoanOverrides(dialString string) map[string]*mvnotifications.LoanTemplates {
	return map[string]*mvnotifications.LoanTemplates{
		"en": {
			Rejected: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Your loan request for %s %.2f was not approved. Reason: %s. "+
					"Dial %s for more info.",
					n.DisplayCurrency, n.DisplayAmount, n.Reason, dialString)
			},
			RepaymentSoon: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Reminder: Your loan payment of %s %.2f is due in %d days (Ref: %s). "+
					"Dial %s to pay.",
					n.DisplayCurrency, n.DisplayAmount, mvnotifications.DaysUntilDue(n), n.LoanReference, dialString)
			},
			RepaymentExpired: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Your repayment request for loan %s has expired. Nothing was paid. "+
					"Dial %s to start over.",
					n.LoanReference, dialString)
			},
			CashPickupCancelled: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Your cash-pickup loan (Ref: %s) was cancelled and the funds returned. "+
					"You owe nothing. Dial %s to request again.",
					n.LoanReference, dialString)
			},
		},
		"sw": {
			Rejected: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Ombi lako la mkopo wa %s %.2f halikuidhinishwa. Sababu: %s. "+
					"Piga %s kwa maelezo zaidi.",
					n.DisplayCurrency, n.DisplayAmount, n.Reason, dialString)
			},
			RepaymentSoon: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Kumbusho: Malipo yako ya mkopo ya %s %.2f yanastahili kwa siku %d (Kumb: %s). "+
					"Piga %s kulipa.",
					n.DisplayCurrency, n.DisplayAmount, mvnotifications.DaysUntilDue(n), n.LoanReference, dialString)
			},
			RepaymentExpired: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Ombi lako la kulipa mkopo %s limeisha muda. Hakuna kilicholipwa. "+
					"Piga %s kuanza upya.",
					n.LoanReference, dialString)
			},
			CashPickupCancelled: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Mkopo wako wa kuchukua fedha (Kumb: %s) umeghairiwa na fedha zimerudishwa. "+
					"Hudaiwi chochote. Piga %s kuomba tena.",
					n.LoanReference, dialString)
			},
		},
		"fr": {
			Rejected: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Votre demande de pret de %s %.2f n'a pas été approuvée. Raison: %s. "+
					"Composez %s pour plus d'informations.",
					n.DisplayCurrency, n.DisplayAmount, n.Reason, dialString)
			},
			RepaymentSoon: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Rappel: Votre paiement de pret de %s %.2f est à payer dans %d jours (Réf: %s). "+
					"Composez %s pour payer.",
					n.DisplayCurrency, n.DisplayAmount, mvnotifications.DaysUntilDue(n), n.LoanReference, dialString)
			},
			RepaymentExpired: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Votre demande de remboursement du pret %s a expire. Rien n'a ete paye. "+
					"Composez %s pour recommencer.",
					n.LoanReference, dialString)
			},
			CashPickupCancelled: func(n contracts.LoanNotification) string {
				return fmt.Sprintf("Votre pret à retrait espèces (Réf: %s) a été annulé et les fonds retournés. "+
					"Vous ne devez rien. Composez %s pour redemander.",
					n.LoanReference, dialString)
			},
		},
	}
}
