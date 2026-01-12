package jobs

// Task type constants for credit module
const (
	// Reminder tasks
	TypeRepaymentReminder = "credit:reminder:repayment"
)

// Payloads for different task types

// RepaymentReminderPayload for sending repayment reminders
type RepaymentReminderPayload struct {
	// Empty - processes all upcoming repayments
}
