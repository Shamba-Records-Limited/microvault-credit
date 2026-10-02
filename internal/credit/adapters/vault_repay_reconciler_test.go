package adapters

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
)

type fakeVaultReconciler struct {
	err   error
	calls []string
}

func (f *fakeVaultReconciler) ReconcileVaultRepay(_ context.Context, l *models.Loan) error {
	f.calls = append(f.calls, l.ID)
	return f.err
}

func TestReconcileDrive_RetriesAFailedRepay(t *testing.T) {
	rec := &fakeVaultReconciler{}
	d := &VaultRepayReconcileDriver{repo: &vaultRepayRepo{}, reconciler: rec, logger: slog.New(slog.DiscardHandler)}

	l := borrowedLoan(5)
	l.VaultRepayStatus = lo.ToPtr(models.VaultRepayStatusFailed)
	d.Drive(t.Context(), l)

	assert.Equal(t, []string{"loan-1"}, rec.calls)
}

func TestReconcileDrive_StalePendingClaimIsParkedNotRetried(t *testing.T) {
	repo := &vaultRepayRepo{}
	rec := &fakeVaultReconciler{}
	alerts := &recordingAlerts{}
	d := &VaultRepayReconcileDriver{repo: repo, reconciler: rec, alerts: alerts, logger: slog.New(slog.DiscardHandler)}

	l := borrowedLoan(1)
	l.VaultRepayStatus = lo.ToPtr(models.VaultRepayStatusPending)
	l.VaultRepayAttemptedAt = lo.ToPtr(time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC))
	d.Drive(t.Context(), l)

	assert.Empty(t, rec.calls)
	assert.Equal(t, []map[string]any{{"vault_repay_status": models.VaultRepayStatusUnknown}}, repo.fields)
	assert.Equal(t, []string{"Vault repay claim stale"}, alerts.subjects)
}
