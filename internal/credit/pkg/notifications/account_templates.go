package notifications

import (
	"fmt"

	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	mvnotifications "github.com/Shamba-Records-Limited/microvault/pkg/notifications"
)

// brandName is the product name shown in account and PIN SMS. Unlike the USSD
// service code it is the same in every environment, so it stays in the copy.
const brandName = "Shamba Records"

// AccountOverrides returns the Shamba Records account and PIN copy, keyed by
// ISO language code. Every field is overridden here: the platform defaults name
// no product, and the registration message walks the user through this app's
// own menu structure.
func AccountOverrides(dialString string) map[string]*mvnotifications.AccountTemplates {
	return map[string]*mvnotifications.AccountTemplates{
		"en": {
			RegistrationSuccess: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("Welcome to %s, %s! Your account is active and your PIN is set. "+
					"Dial %s to request your first loan.\n\n"+
					"Next: My Account > PIN Manager > Security Questions - so you can recover your "+
					"account if you lose your phone. Add My Details for faster cash pickup.",
					brandName, n.FullName, dialString)
			},
			RegistrationFailed: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("Your %s registration could not be completed. Reason: %s. "+
					"Please try again or contact support.", brandName, n.Reason)
			},
			WrongAttempt: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("ALERT: An incorrect PIN was entered on your %s account. "+
					"%d attempt(s) remaining before your account is locked.", brandName, n.RemainingAttempts)
			},
			AccountLocked: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("SECURITY: Your %s account has been temporarily locked due to multiple "+
					"failed PIN attempts. Try again in %s or dial %s to reset your PIN.",
					brandName, n.LockedUntil, dialString)
			},
			PINChanged: func(contracts.AccountNotification) string {
				return fmt.Sprintf("Your %s PIN has been changed successfully. If you did not make this "+
					"change, dial %s immediately to reset your PIN.", brandName, dialString)
			},
			PINChangeFailed: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("A PIN change attempt on your %s account was unsuccessful. Reason: %s. "+
					"If this was not you, please reset your PIN immediately.", brandName, n.Reason)
			},
			PINReset: func(contracts.AccountNotification) string {
				return fmt.Sprintf("Your %s PIN has been reset successfully. "+
					"You can now access your account with your new PIN.", brandName)
			},
			PINResetFailed: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("A PIN reset attempt on your %s account failed. Reason: %s. "+
					"Please try again or contact support.", brandName, n.Reason)
			},
		},
		"sw": {
			RegistrationSuccess: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("Karibu %s, %s! Akaunti yako iko hai na PIN yako imewekwa. "+
					"Piga %s kuomba mkopo wako wa kwanza.\n\n"+
					"Ifuatayo: Akaunti Yangu > Dhibiti PIN > Maswali ya Usalama - ili uweze kurejesha "+
					"akaunti yako ukipoteza simu. Weka Maelezo Yangu kwa kuchukua pesa haraka.",
					brandName, n.FullName, dialString)
			},
			RegistrationFailed: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("Usajili wako wa %s haukukamilika. Sababu: %s. "+
					"Tafadhali jaribu tena au wasiliana na msaada.", brandName, n.Reason)
			},
			WrongAttempt: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("TAHADHARI: PIN isiyo sahihi iliwekwa kwenye akaunti yako ya %s. "+
					"Majaribio %d yamebaki kabla akaunti yako kufungwa.", brandName, n.RemainingAttempts)
			},
			AccountLocked: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("USALAMA: Akaunti yako ya %s imefungwa kwa muda kutokana na majaribio "+
					"mengi ya PIN yaliyoshindwa. Jaribu tena baada ya %s au piga %s kuweka upya PIN.",
					brandName, n.LockedUntil, dialString)
			},
			PINChanged: func(contracts.AccountNotification) string {
				return fmt.Sprintf("PIN yako ya %s imebadilishwa. Kama hukufanya mabadiliko haya, "+
					"piga %s mara moja kuweka upya PIN yako.", brandName, dialString)
			},
			PINChangeFailed: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("Jaribio la kubadilisha PIN kwenye akaunti yako ya %s halikufanikiwa. "+
					"Sababu: %s. Kama hukuwa wewe, tafadhali weka upya PIN yako mara moja.", brandName, n.Reason)
			},
			PINReset: func(contracts.AccountNotification) string {
				return fmt.Sprintf("PIN yako ya %s imewekwa upya. "+
					"Sasa unaweza kufikia akaunti yako kwa PIN yako mpya.", brandName)
			},
			PINResetFailed: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("Jaribio la kuweka upya PIN kwenye akaunti yako ya %s limeshindwa. "+
					"Sababu: %s. Tafadhali jaribu tena au wasiliana na msaada.", brandName, n.Reason)
			},
		},
		"fr": {
			RegistrationSuccess: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("Bienvenue à %s, %s! Votre compte est actif et votre PIN est défini. "+
					"Composez %s pour demander votre premier pret.\n\n"+
					"Ensuite: Mon Compte > Gérer PIN > Questions de Sécurité - pour récupérer votre "+
					"compte si vous perdez votre téléphone. Ajoutez Mes Infos pour un retrait plus rapide.",
					brandName, n.FullName, dialString)
			},
			RegistrationFailed: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("Votre inscription à %s n'a pas pu etre complétée. Raison: %s. "+
					"Veuillez réessayer ou contacter le support.", brandName, n.Reason)
			},
			WrongAttempt: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("ALERTE: Un PIN incorrect a été saisi sur votre compte %s. "+
					"%d tentative(s) restante(s) avant le verrouillage de votre compte.", brandName, n.RemainingAttempts)
			},
			AccountLocked: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("SÉCURITÉ: Votre compte %s a été temporairement verrouillé suite à "+
					"plusieurs échecs de PIN. Réessayez dans %s ou composez %s pour réinitialiser votre PIN.",
					brandName, n.LockedUntil, dialString)
			},
			PINChanged: func(contracts.AccountNotification) string {
				return fmt.Sprintf("Votre PIN %s a été modifié avec succès. Si vous n'etes pas à l'origine "+
					"de ce changement, composez %s immédiatement pour réinitialiser votre PIN.",
					brandName, dialString)
			},
			PINChangeFailed: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("Une tentative de changement de PIN sur votre compte %s a échoué. "+
					"Raison: %s. Si ce n'était pas vous, réinitialisez votre PIN immédiatement.", brandName, n.Reason)
			},
			PINReset: func(contracts.AccountNotification) string {
				return fmt.Sprintf("Votre PIN %s a été réinitialisé avec succès. "+
					"Vous pouvez maintenant accéder à votre compte avec votre nouveau PIN.", brandName)
			},
			PINResetFailed: func(n contracts.AccountNotification) string {
				return fmt.Sprintf("Une tentative de réinitialisation de PIN sur votre compte %s a échoué. "+
					"Raison: %s. Veuillez réessayer ou contacter le support.", brandName, n.Reason)
			},
		},
	}
}
