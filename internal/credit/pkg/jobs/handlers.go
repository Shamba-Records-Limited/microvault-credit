package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	pkgjobs "github.com/Shamba-Records-Limited/microvault/pkg/jobs"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// JobHandlers contains all job handlers for the credit module
type JobHandlers struct {
	// Services
	loanNotifier contracts.LoanNotifier
	redisClient  *redis.Client

	// Repositories (add these as you create them)
	// repaymentRepo *repository.RepaymentRepository
	// loanRepo      *repository.LoanRepository
	// userRepo      *repository.UserRepository
}

// NewJobHandlers creates a new job handlers instance
func NewJobHandlers(
	loanNotifier contracts.LoanNotifier,
	redisClient *redis.Client,
) *JobHandlers {
	return &JobHandlers{
		loanNotifier: loanNotifier,
		redisClient:  redisClient,
	}
}

// RegisterHandlers registers all job handlers with the scheduler
func (h *JobHandlers) RegisterHandlers(scheduler *pkgjobs.Scheduler) {
	// Reminder handlers
	scheduler.RegisterHandler(TypeRepaymentReminder, h.HandleRepaymentReminder)

	log.Println("All credit job handlers registered successfully")
}

// =====================================================
// Reminder Handlers
// =====================================================

// HandleRepaymentReminder sends repayment reminders for upcoming and overdue loans
// Adapted from RepaymentReminderJob to work with asynq task scheduler
func (h *JobHandlers) HandleRepaymentReminder(task *asynq.Task) error {
	ctx := context.Background()
	log.Println("Starting repayment reminder job...")

	var payload RepaymentReminderPayload
	if err := pkgjobs.ParsePayload(task, &payload); err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	now := time.Now()
	remindersSent := 0

	// TODO: Uncomment when repositories are available
	// Get upcoming repayments (next 7 days)
	// sevenDaysFromNow := now.AddDate(0, 0, 7)
	// upcomingRepayments, err := h.repaymentRepo.GetUpcoming(ctx, now, sevenDaysFromNow)
	// if err != nil {
	// 	return fmt.Errorf("failed to get upcoming repayments: %w", err)
	// }

	// Placeholder - replace with actual repository call
	upcomingRepayments := []models.Repayment{}

	log.Printf("Found %d upcoming repayments to check", len(upcomingRepayments))

	// Process each repayment
	for _, repayment := range upcomingRepayments {
		if repayment.Status == models.RepaymentStatusPaid {
			continue
		}

		// Calculate days until due
		daysUntilDue := int(time.Until(repayment.DueDate).Hours() / 24)

		// Only send reminders at specific intervals: 7, 3, 1 days before
		shouldSend := false
		reminderType := ""

		switch daysUntilDue {
		case 7:
			shouldSend = true
			reminderType = "7d"
		case 3:
			shouldSend = true
			reminderType = "3d"
		case 1:
			shouldSend = true
			reminderType = "1d"
		}

		if !shouldSend {
			continue
		}

		// Check if reminder already sent (using cache to track)
		cacheKey := fmt.Sprintf("reminder_sent:%s:%s", repayment.ID, reminderType)
		_, err := h.redisClient.Get(ctx, cacheKey).Result()
		if err == nil {
			// Reminder already sent for this interval (key exists)
			continue
		}

		// TODO: Uncomment when repositories are available
		// Get loan and user details
		// loan, err := h.loanRepo.GetByID(ctx, repayment.LoanID)
		// if err != nil {
		// 	log.Printf("Warning: failed to get loan %s: %v", repayment.LoanID, err)
		// 	continue
		// }

		// user, err := h.userRepo.GetByID(ctx, repayment.UserID)
		// if err != nil {
		// 	log.Printf("Warning: failed to get user %s: %v", repayment.UserID, err)
		// 	continue
		// }

		// if user.MobileNumber == "" || loan.LoanReference == nil {
		// 	continue
		// }

		// Placeholder values - replace with actual data
		loanRef := "LOAN-001"
		phoneNumber := "+254712345678"

		// Calculate amount due in KES
		amountDueKES := float64(repayment.AmountDue+repayment.LateFee-repayment.AmountPaid) / 1000000.0 * 150.0

		// Send reminder
		// phoneNumber := user.MobileCountryCode + user.MobileNumber
		dueDate := repayment.DueDate
		if err := h.loanNotifier.NotifyRepaymentReminder(ctx, contracts.LoanNotification{
			LoanReference:   loanRef,
			PhoneNumber:     phoneNumber,
			DisplayAmount:   amountDueKES,
			DisplayCurrency: "KES",
			DueDate:         &dueDate,
		}); err != nil {
			log.Printf("Warning: failed to send reminder for repayment %s: %v", repayment.ID, err)
			continue
		}

		// Mark reminder as sent (cache for 24 hours)
		if err := h.redisClient.Set(ctx, cacheKey, "1", 24*time.Hour).Err(); err != nil {
			log.Printf("Warning: failed to cache reminder status: %v", err)
		}

		remindersSent++
		log.Printf("Sent %d-day reminder for loan %s (repayment %s)", daysUntilDue, loanRef, repayment.ID)
	}

	// Also send overdue reminders (once per day for overdue loans)
	// TODO: Uncomment when repository is available
	// overdueRepayments, err := h.repaymentRepo.GetOverdue(ctx, now)
	// if err != nil {
	// 	log.Printf("Warning: failed to get overdue repayments: %v", err)
	// } else {

	overdueRepayments := []models.Repayment{} // Placeholder
	log.Printf("Found %d overdue repayments to remind", len(overdueRepayments))

	for _, repayment := range overdueRepayments {
		// Check if overdue reminder already sent today
		cacheKey := fmt.Sprintf("reminder_sent:%s:overdue:%s", repayment.ID, now.Format("2006-01-02"))
		_, err := h.redisClient.Get(ctx, cacheKey).Result()
		if err == nil {
			// Reminder already sent today (key exists)
			continue
		}

		// TODO: Uncomment when repositories are available
		// Get loan and user details
		// loan, err := h.loanRepo.GetByID(ctx, repayment.LoanID)
		// if err != nil {
		// 	log.Printf("Warning: failed to get loan %s: %v", repayment.LoanID, err)
		// 	continue
		// }

		// user, err := h.userRepo.GetByID(ctx, repayment.UserID)
		// if err != nil {
		// 	log.Printf("Warning: failed to get user %s: %v", repayment.UserID, err)
		// 	continue
		// }

		// if user.MobileNumber == "" || loan.LoanReference == nil {
		// 	continue
		// }

		// Placeholder values
		loanRef := "LOAN-001"
		phoneNumber := "+254712345678"

		// Calculate amount due in KES
		amountDueKES := float64(repayment.AmountDue+repayment.LateFee-repayment.AmountPaid) / 1000000.0 * 150.0

		// Send overdue reminder
		// phoneNumber := user.MobileCountryCode + user.MobileNumber
		overdueDueDate := repayment.DueDate
		if err := h.loanNotifier.NotifyRepaymentReminder(ctx, contracts.LoanNotification{
			LoanReference:   loanRef,
			PhoneNumber:     phoneNumber,
			DisplayAmount:   amountDueKES,
			DisplayCurrency: "KES",
			DueDate:         &overdueDueDate,
		}); err != nil {
			log.Printf("Warning: failed to send overdue reminder for repayment %s: %v", repayment.ID, err)
			continue
		}

		// Mark reminder as sent (cache for 24 hours)
		if err := h.redisClient.Set(ctx, cacheKey, "1", 24*time.Hour).Err(); err != nil {
			log.Printf("Warning: failed to cache reminder status: %v", err)
		}

		remindersSent++
		log.Printf("Sent overdue reminder for loan %s (repayment %s)", loanRef, repayment.ID)
	}

	log.Printf("Repayment reminder job complete: %d reminders sent", remindersSent)
	return nil
}
