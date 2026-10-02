package adapters

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"
	"github.com/samber/oops"

	"github.com/Shamba-Records-Limited/microvault/pkg/alerts"
	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mgpoller"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
)

// vaultRepayReconciler is the part of DisbursementStatusAdapter the reconciler
// drives, so tests can stand in for the on-chain call.
type vaultRepayReconciler interface {
	ReconcileVaultRepay(ctx context.Context, loan *models.Loan) error
	ResolveVaultRepay(ctx context.Context, loan *models.Loan) error
}

// VaultRepayReconcileDriver returns USDC owed to the vault for unwound
// disbursements once the off-ramp pollers have had their settlement window.
// It covers only the vault_repay_* leg; borrower repayments, including those
// settled through the OTC desk, never reach it.
type VaultRepayReconcileDriver struct {
	repo       repository.LoanRepository
	reconciler vaultRepayReconciler
	alerts     mgpoller.AlertService
	logger     *slog.Logger
}

// VaultRepayReconcilerDeps are the collaborators and schedule. Alerts and
// Logger are optional; DB enables the advisory lock that keeps a second
// replica from running the same tick.
type VaultRepayReconcilerDeps struct {
	Repo       repository.LoanRepository
	Reconciler *DisbursementStatusAdapter
	DB         *sql.DB
	Alerts     mgpoller.AlertService
	Logger     *slog.Logger

	Interval         time.Duration
	SettlementWindow time.Duration
	RetryBackoff     time.Duration
	MaxAttempts      int
}

// NewVaultRepayReconcileRunner pairs the driver with its due-set query.
func NewVaultRepayReconcileRunner(deps VaultRepayReconcilerDeps) (*mgpoller.Runner[*models.Loan], error) {
	if deps.Repo == nil || deps.Reconciler == nil {
		return nil, oops.In(pkgErrors.DomainOffRamp).Tags("vault-repay-reconciler").
			Code(pkgErrors.CodeMissingDependency).Errorf("repository and disbursement adapter are required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	driver := &VaultRepayReconcileDriver{
		repo:       deps.Repo,
		reconciler: deps.Reconciler,
		alerts:     deps.Alerts,
		logger:     logger.With("component", "vault_repay_reconciler"),
	}
	return mgpoller.NewRunner[*models.Loan](mgpoller.RunnerDeps[*models.Loan]{
		Direction: "vault-repay-reconciler",
		Interval:  deps.Interval,
		MaxBatch:  50,
		Fetcher: mgpoller.FetchFunc[*models.Loan](func(ctx context.Context, limit int) ([]*models.Loan, error) {
			return deps.Repo.GetDueVaultRepays(ctx, repository.VaultRepayDue{
				Now:              time.Now(),
				SettlementWindow: deps.SettlementWindow,
				RetryBackoff:     deps.RetryBackoff,
				MaxAttempts:      deps.MaxAttempts,
			}, limit)
		}),
		Driver:        driver,
		DB:            deps.DB,
		Logger:        logger,
		LoanID:        func(l *models.Loan) string { return l.ID },
		LoanReference: func(l *models.Loan) string { return lo.FromPtr(l.LoanReference) },
	}), nil
}

// Drive settles an unconfirmed repay from the ledger when its transaction
// was recorded, and otherwise retries a failed one. A claim still pending
// past the settlement window with no recorded transaction died before
// signing or predates recording, so it is parked as unknown for an operator.
func (d *VaultRepayReconcileDriver) Drive(ctx context.Context, l *models.Loan) {
	if l == nil {
		return
	}
	if l.VaultRepayPendingTxHash != nil && *l.VaultRepayPendingTxHash != "" {
		if err := d.reconciler.ResolveVaultRepay(ctx, l); err != nil {
			d.logger.WarnContext(ctx, "vault repay resolution failed", "loan_id", l.ID, "error", err)
		}
		return
	}
	if l.VaultRepayStatus != nil && *l.VaultRepayStatus == models.VaultRepayStatusPending {
		d.parkStaleClaim(ctx, l)
		return
	}

	err := d.reconciler.ReconcileVaultRepay(ctx, l)
	switch {
	case err == nil:
		d.logger.InfoContext(ctx, "vault repay reconciled", "loan_id", l.ID)
	case errors.Is(err, contracts.ErrVaultRepayDeferred):
		d.logger.InfoContext(ctx, "vault repay claimed elsewhere, skipping", "loan_id", l.ID)
	default:
		d.logger.WarnContext(ctx, "vault repay reconcile attempt failed", "loan_id", l.ID, "error", err)
	}
}

func (d *VaultRepayReconcileDriver) parkStaleClaim(ctx context.Context, l *models.Loan) {
	if err := d.repo.UpdateFields(ctx, l.ID, map[string]any{
		"vault_repay_status": models.VaultRepayStatusUnknown,
	}); err != nil {
		d.logger.ErrorContext(ctx, "could not park stale vault repay claim", "loan_id", l.ID, "error", err)
		return
	}
	msg := fmt.Sprintf("Loan %s: vault repay claim has been pending since %v with no result recorded. "+
		"Verify on-chain, then set vault_repay_tx_hash or reset vault_repay_status to failed.",
		l.ID, lo.FromPtr(l.VaultRepayAttemptedAt).Format(time.RFC3339))
	alerts.Raise(ctx, d.alerts, d.logger, "Vault repay claim stale", msg)
}
