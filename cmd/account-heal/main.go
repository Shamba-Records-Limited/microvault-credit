// Command account-heal reports on, and optionally heals, one borrower's child
// Stellar account.
//
// By default it changes nothing: it prints the account row, whether the
// address derived from the stored index matches the stored address, and
// whether the account exists on-chain. With --apply it runs the same
// EnsureOnChainAccount the account chain reconciler and the loan path use, so
// there is one heal implementation. Accounts in conflict (a reused derivation
// index) are refused; they must be re-issued at a fresh index.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/joho/godotenv/autoload"

	"github.com/Shamba-Records-Limited/microvault/pkg/account"
	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	"github.com/Shamba-Records-Limited/microvault/pkg/logging"
	ussdadapters "github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd/adapters"
	"github.com/Shamba-Records-Limited/microvault/pkg/models"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	"github.com/Shamba-Records-Limited/microvault/pkg/user"
	"github.com/Shamba-Records-Limited/microvault/platform/database"
)

// Exit codes: 0 healthy or healed, 1 error, 2 refused or needs attention.
const (
	exitError     = 1
	exitAttention = 2
)

func main() {
	logging.Setup()

	ref := flag.String("account", "", "account id, Stellar address, or the user's mobile number as stored")
	apply := flag.Bool("apply", false, "run EnsureOnChainAccount; without it nothing is changed")
	flag.Usage = printUsage
	flag.Parse()
	if *ref == "" {
		printUsage()
		os.Exit(exitError)
	}

	cfg, err := config.New()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	os.Exit(run(cfg, strings.TrimSpace(*ref), *apply))
}

func run(cfg *config.Config, ref string, apply bool) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := database.GetConnection("account-heal", &cfg.Postgres)
	if err != nil {
		fmt.Printf("Could not connect to the database: %v\n", err)
		return exitError
	}
	coreRepos, err := corerepository.NewRepositories(db)
	if err != nil {
		fmt.Printf("Could not build repositories: %v\n", err)
		return exitError
	}

	stellarSvc := stellar.NewService(
		cfg.Stellar.NewRpcClient(),
		cfg.Stellar.NetworkPassphrase,
		cfg.Stellar.TreasurySecretKey,
		cfg.Stellar.AdminSecretKey,
		cfg.Stellar.ContractID,
		cfg.Stellar.USDCIssuer,
	)
	userAdapter, err := ussdadapters.NewUserServiceAdapter(
		user.NewService(coreRepos.User),
		account.NewService(coreRepos.Account, coreRepos.User),
		stellarSvc,
		db,
		ussdadapters.WalletConfig{
			TreasuryPrivateKey: cfg.Stellar.TreasurySecretKey,
			USDCIssuer:         cfg.Stellar.USDCIssuer,
			EnableMultiSig:     cfg.Stellar.EnableMultiSig,
			LowThreshold:       cfg.Stellar.MultiSigLowThreshold,
			MediumThreshold:    cfg.Stellar.MultiSigMediumThreshold,
			HighThreshold:      cfg.Stellar.MultiSigHighThreshold,
		},
		nil,
		nil,
	)
	if err != nil {
		fmt.Printf("Could not build the user service adapter: %v\n", err)
		return exitError
	}

	acct, err := resolveAccount(ctx, coreRepos, ref)
	if err != nil {
		fmt.Printf("Could not find an account for %q: %v\n", ref, err)
		return exitError
	}

	derived, deriveErr := userAdapter.DeriveChildAddress(acct.AccountIndex)
	onChain, existsErr := stellarSvc.AccountExists(ctx, acct.PublicKey)

	fmt.Println("Account chain state")
	fmt.Printf("  account:          %s\n", acct.ID)
	fmt.Printf("  user:             %s\n", acct.UserID)
	fmt.Printf("  address:          %s\n", acct.PublicKey)
	fmt.Printf("  index:            %d\n", acct.AccountIndex)
	fmt.Printf("  chain_status:     %s\n", acct.ChainStatus)
	fmt.Printf("  chain_attempts:   %d\n", acct.ChainAttempts)
	fmt.Printf("  chain_checked_at: %s\n", formatTime(acct.ChainCheckedAt))
	switch {
	case deriveErr != nil:
		fmt.Printf("  derived address:  error: %v\n", deriveErr)
	case derived == acct.PublicKey:
		fmt.Printf("  derived address:  matches\n")
	default:
		fmt.Printf("  derived address:  %s (MISMATCH: the seed or the stored index is wrong)\n", derived)
	}
	if existsErr != nil {
		fmt.Printf("  on-chain:         could not check: %v\n", existsErr)
	} else {
		fmt.Printf("  on-chain:         %t\n", onChain)
	}

	if acct.ChainStatus == models.ChainStatusConflict {
		fmt.Println("\nRefused: this address was derived from a reused index and belongs to another account.")
		fmt.Println("Re-issue the account at a fresh index; healing would lend against someone else's account.")
		return exitAttention
	}
	if deriveErr != nil || derived != acct.PublicKey {
		fmt.Println("\nRefused: the stored address does not match its index, so nothing can be created safely.")
		return exitAttention
	}

	if !apply {
		if acct.ChainStatus == models.ChainStatusConfirmed && onChain {
			fmt.Println("\nHealthy. Nothing to do.")
			return 0
		}
		fmt.Println("\nNothing was changed. Re-run with --apply to create or confirm the on-chain account.")
		return exitAttention
	}

	if err := userAdapter.EnsureOnChainAccount(ctx, acct.AccountIndex, acct.PublicKey); err != nil {
		fmt.Printf("\nHeal failed: %v\n", err)
		if errors.Is(err, account.ErrChainCheckUnavailable) {
			fmt.Println("The RPC or database was unreachable; nothing was attempted on-chain. Try again shortly.")
		}
		return exitError
	}
	if err := coreRepos.Account.RecordChainCheck(ctx, acct.ID, 0, time.Now()); err != nil {
		fmt.Printf("Healed, but the attempt counter could not be reset: %v\n", err)
	}

	healed, err := coreRepos.Account.GetByID(ctx, acct.ID)
	if err != nil {
		fmt.Println("\nHealed. Could not re-read the row to show its new status.")
		return 0
	}
	fmt.Printf("\nHealed. chain_status is now %s.\n", healed.ChainStatus)
	return 0
}

// resolveAccount accepts an account id, a Stellar address (G…, 56 chars), or
// a mobile number as stored on the user row.
func resolveAccount(ctx context.Context, repos *corerepository.Repositories, ref string) (*models.Account, error) {
	if len(ref) == 56 && strings.HasPrefix(ref, "G") {
		return repos.Account.GetByPublicKey(ctx, ref)
	}
	if _, err := uuid.Parse(ref); err == nil {
		return repos.Account.GetByID(ctx, ref)
	}
	u, err := repos.User.GetByMobileNumber(ctx, ref)
	if err != nil {
		return nil, err
	}
	return repos.Account.GetByUserID(ctx, u.ID)
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return t.UTC().Format(time.RFC3339)
}

func printUsage() {
	fmt.Println("Account Chain Heal CLI")
	fmt.Println("\nUsage:")
	fmt.Println("  account-heal --account <account-id | stellar-address | mobile-number> [--apply]")
	fmt.Println("\nWithout --apply nothing is changed; the account's chain state is reported.")
	fmt.Println("\nExamples:")
	fmt.Println("  account-heal --account GABC...XYZ")
	fmt.Println("  account-heal --account 0192f3a4-... --apply")
	fmt.Println("\nExit codes: 0 healthy or healed, 1 error, 2 refused or needs attention.")
}
