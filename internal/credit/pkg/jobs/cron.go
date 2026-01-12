package jobs

import (
	"log"

	pkgjobs "github.com/Shamba-Records-Limited/Microvault/pkg/jobs"
)

// RegisterCronJobs registers all scheduled cron jobs for the credit module
func RegisterCronJobs(scheduler *pkgjobs.Scheduler) error {
	// Repayment reminders - Run daily at 9 AM
	task, err := pkgjobs.NewTask(TypeRepaymentReminder, RepaymentReminderPayload{})
	if err != nil {
		return err
	}
	if err := scheduler.RegisterCronJob(
		"0 9 * * *", // Every day at 9 AM
		TypeRepaymentReminder,
		task.Payload(),
		pkgjobs.CriticalQueue,
		pkgjobs.MaxRetry3,
	); err != nil {
		return err
	}
	log.Println("Registered cron job: Repayment reminders (daily at 9 AM)")

	log.Println("All credit module cron jobs registered successfully")
	return nil
}
