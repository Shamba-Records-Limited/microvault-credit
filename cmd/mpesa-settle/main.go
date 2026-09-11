// Command mpesa-settle writes the vault leg for an M-Pesa repayment settled
// by hand.
//
// M-Pesa collections convert to USDC through an OTC desk today, not an
// automated anchor flow — cheaper than per-transaction on-ramp fees, but it
// means nothing polls for "the money arrived" the way MoneyGram's SEP-24
// poller does (see config.MpesaConfig.SettlementMode). This command is that
// missing step: once the OTC-desk USDC has landed in treasury — verified
// out-of-band, before running this — it executes the on-chain repay_for call
// and records the result.
//
// Amount and borrower address come from the loan row, never the command
// line: RepayForBorrower executes a real transfer, so nothing here should let
// a typed figure move the wrong amount on-chain.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"

	_ "github.com/joho/godotenv/autoload"

	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/sms"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/sms/providers/africastalking"
	ussdadapters "github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd/adapters"
	mvnotifications "github.com/Shamba-Records-Limited/microvault/pkg/notifications"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/yellowcard"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	"github.com/Shamba-Records-Limited/microvault/platform/database"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/adapters"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfg, err := config.New()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	switch os.Args[1] {
	case "settle":
		if err := settle(cfg, os.Args[2:]); err != nil {
			log.Fatalf("%v", err)
		}
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// settle returns an error rather than calling log.Fatalf once the advisory
// lock (below) is held, so the deferred unlock always runs — os.Exit, which
// Fatalf calls, skips every pending defer in the program, not just the ones
// in the function that called it.
func settle(cfg *config.Config, args []string) error {
	var confirm bool
	var loanID string
	for _, a := range args {
		if a == "--confirm" || a == "-confirm" {
			confirm = true
			continue
		}
		if loanID == "" {
			loanID = a
		}
	}
	if loanID == "" {
		log.Fatal("Usage: mpesa-settle settle <loan-id> [--confirm]")
	}

	db, err := database.GetConnection("mpesa-settle", &cfg.Postgres)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	repos, err := repository.NewRepositories(db)
	if err != nil {
		log.Fatalf("Failed to initialize repositories: %v", err)
	}
	coreRepos, err := corerepository.NewRepositories(db)
	if err != nil {
		log.Fatalf("Failed to initialize core repositories: %v", err)
	}
	loanSvc := loan.NewService(repos.Loan, cfg.Payments.LoanReferencePrefix)

	loanRow, err := repos.Loan.GetByID(context.Background(), loanID)
	if err != nil {
		log.Fatalf("Could not load loan %s: %v", loanID, err)
	}
	if loanRow.RepaymentProvider != models.LoanRepaymentProviderMpesa {
		log.Fatalf("Loan %s is not on the M-Pesa rail (repayment_provider=%q)", loanID, loanRow.RepaymentProvider)
	}
	if loanRow.RepaymentStatus != models.LoanRepaymentStatusFundsReceived {
		log.Fatalf("Loan %s is not awaiting settlement (repayment_status=%q, expected %q)",
			loanID, loanRow.RepaymentStatus, models.LoanRepaymentStatusFundsReceived)
	}
	if loanRow.Account == nil || loanRow.Account.PublicKey == "" {
		log.Fatalf("Loan %s has no borrower account to attribute the repayment to", loanID)
	}
	if loanRow.RepaymentPayoffStroops == nil || *loanRow.RepaymentPayoffStroops <= 0 {
		log.Fatalf("Loan %s has no positive frozen payoff to settle", loanID)
	}
	amountStroops := *loanRow.RepaymentPayoffStroops

	fmt.Println("M-Pesa manual settlement")
	fmt.Printf("  loan:             %s\n", loanID)
	fmt.Printf("  reference:        %s\n", derefStr(loanRow.LoanReference))
	fmt.Printf("  borrower:         %s\n", loanRow.Account.PublicKey)
	fmt.Printf("  amount (stroops): %d\n", amountStroops)
	fmt.Println("\nPrecondition, not enforced here: the OTC-desk USDC deposit for this amount has already landed in treasury.")

	if !confirm {
		fmt.Println("\nNothing was sent. Re-run with --confirm to execute the on-chain repay.")
		return nil
	}

	// A Postgres advisory lock, keyed on the loan ID, closes the gap between
	// the funds_received check above and the on-chain call below: without it,
	// two operators (or one, running this twice by mistake) racing on the
	// same loan could both pass the check and both execute RepayForBorrower,
	// which is a real transfer with no idempotency key of its own. Session-
	// scoped rather than transaction-scoped — this is a one-shot process
	// holding one connection for the command's whole lifetime, not a pooled
	// ticker, so there is no connection-pinning concern to avoid.
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("could not access the underlying database connection: %w", err)
	}
	var acquired bool
	if err := sqlDB.QueryRowContext(context.Background(), "SELECT pg_try_advisory_lock(hashtext($1))", loanID).Scan(&acquired); err != nil {
		return fmt.Errorf("could not acquire the settlement lock: %w", err)
	}
	if !acquired {
		return fmt.Errorf("another settlement is already in progress for loan %s", loanID)
	}
	defer func() {
		if _, err := sqlDB.Exec("SELECT pg_advisory_unlock(hashtext($1))", loanID); err != nil {
			log.Printf("warning: could not release the settlement lock for loan %s: %v", loanID, err)
		}
	}()

	// Re-check after acquiring the lock: the loan may have been settled by
	// another run between the first read above and the lock being granted.
	loanRow, err = repos.Loan.GetByID(context.Background(), loanID)
	if err != nil {
		return fmt.Errorf("could not reload loan %s: %w", loanID, err)
	}
	if loanRow.RepaymentStatus != models.LoanRepaymentStatusFundsReceived {
		return fmt.Errorf("loan %s is no longer awaiting settlement (repayment_status=%q) — another run likely settled it first",
			loanID, loanRow.RepaymentStatus)
	}
	if loanRow.Account == nil || loanRow.Account.PublicKey == "" {
		return fmt.Errorf("loan %s has no borrower account to attribute the repayment to", loanID)
	}
	if loanRow.RepaymentPayoffStroops == nil || *loanRow.RepaymentPayoffStroops <= 0 {
		return fmt.Errorf("loan %s has no positive frozen payoff to settle", loanID)
	}
	amountStroops = *loanRow.RepaymentPayoffStroops

	rpcClient := cfg.Stellar.NewRpcClient()
	stellarSvc := stellar.NewService(
		rpcClient,
		cfg.Stellar.NetworkPassphrase,
		cfg.Stellar.TreasurySecretKey,
		cfg.Stellar.AdminSecretKey,
		cfg.Stellar.ContractID,
		cfg.Stellar.USDCIssuer,
	)

	// Reused as-is despite the name: RepayForBorrower/MarkSettled do nothing
	// MoneyGram-specific, and the transaction-recording path they share
	// already reads the provider label off the loan row rather than assuming
	// MoneyGram. Extracting a provider-agnostic settlement type is a
	// follow-up, not a blocker.
	depositAdapter, err := adapters.NewMoneyGramDepositAdapter(adapters.DepositAdapterDeps{
		Repo:       repos.Loan,
		LoanSvc:    loanSvc,
		StellarSvc: stellarSvc,
		Logger:     slog.Default(),
	})
	if err != nil {
		return fmt.Errorf("could not construct the settlement adapter: %w", err)
	}

	// A YellowCard-only offramp registry, just deep enough for
	// RepaymentNotifierAdapter's FX lookup (Quote never touches Treasury, so
	// nil is fine here — this registry never calls Initiate). Registered
	// under its real ProviderID so it also serves as the disbursing
	// provider's own rate whenever a settled loan's RampProvider is
	// "yellowcard", not just the fallback.
	ycAdapter := yellowcard.NewYellowcardAdapter(
		cfg.Payments.YellowCard.PublicKey,
		cfg.Payments.YellowCard.SecretKey,
		cfg.Payments.YellowCard.BaseURL,
	)
	ycOffRamp := ussdadapters.NewYellowCardOffRampAdapter(ussdadapters.YellowCardOffRampConfig{
		Adapter:      ycAdapter,
		BusinessID:   cfg.Payments.YellowCard.BusinessID,
		BusinessName: cfg.Payments.YellowCard.BusinessName,
		Logger:       slog.Default(),
	})
	offRampRegistry := offramp.NewRegistry()
	if err := offRampRegistry.Register(ycOffRamp); err != nil {
		return fmt.Errorf("could not register the YellowCard FX source: %w", err)
	}

	// Minimal notification stack otherwise: no language resolver (defaults
	// to English) and no link shortener (nil is accepted — NotifyLoanRepaid
	// never renders a link, so there is nothing for it to shorten). This is
	// a one-shot CLI with no user service to back a language lookup, unlike
	// cmd/credit.
	atSMSAdapter := africastalking.NewAfricasTalkingSMSAdapter(
		cfg.Mobile.AfricasTalking.Username,
		cfg.Mobile.AfricasTalking.APIKey,
		cfg.Mobile.AfricasTalking.BaseURL,
		cfg.Mobile.AfricasTalking.HTTPTimeout,
	)
	smsService := sms.NewSMSService()
	smsService.RegisterProvider("africastalking", atSMSAdapter)
	atProvider, _ := smsService.GetProvider("africastalking")
	smsNotifier := mvnotifications.NewSMSNotifier(atProvider, cfg.Mobile.AfricasTalking.ResolveSenderID())
	loanNotifier, err := mvnotifications.NewSMSLoanNotifier(smsNotifier)
	if err != nil {
		return fmt.Errorf("could not construct the loan notifier: %w", err)
	}
	repaymentNotifier, err := adapters.NewRepaymentNotifierAdapter(repos.Loan, loanNotifier, "", nil, offRampRegistry, slog.Default())
	if err != nil {
		return fmt.Errorf("could not construct the repayment notifier: %w", err)
	}

	ctx := context.Background()
	txHash, err := depositAdapter.RepayForBorrower(ctx, loanID, loanRow.Account.PublicKey, amountStroops)
	if err != nil {
		return fmt.Errorf("vault repay failed: %w", err)
	}
	if err := depositAdapter.MarkSettled(ctx, loanID, txHash); err != nil {
		return fmt.Errorf("vault repay succeeded (tx %s) but recording settlement failed — fix the loan row by hand: %w", txHash, err)
	}

	var amountKES int64
	if loanRow.RepaymentMpesaTransID != nil {
		if obs, err := coreRepos.Mpesa.GetByTransID(ctx, *loanRow.RepaymentMpesaTransID); err == nil {
			amountKES = obs.AmountKes
		}
	}
	if err := repaymentNotifier.NotifyLoanRepaidAmount(loanID, amountKES); err != nil {
		log.Printf("warning: settled but could not notify the borrower: %v", err)
	}

	fmt.Printf("\nSettled. txHash=%s\n", txHash)
	return nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func printUsage() {
	fmt.Println("M-Pesa Manual Settlement CLI")
	fmt.Println("\nUsage:")
	fmt.Println("  mpesa-settle settle <loan-id> [--confirm]")
	fmt.Println("\nExamples:")
	fmt.Println("  mpesa-settle settle 3f8a1c2e-...")
	fmt.Println("  mpesa-settle settle 3f8a1c2e-... --confirm")
}
