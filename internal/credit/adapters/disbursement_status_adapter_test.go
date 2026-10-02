package adapters

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
)

func TestRetryOnNotFound(t *testing.T) {
	notFound := errors.New("not found")
	otherErr := errors.New("something else broke")
	isRace := func(err error) bool { return errors.Is(err, notFound) }

	t.Run("succeeds immediately, no wait", func(t *testing.T) {
		calls := 0
		var waited []time.Duration
		got, err := retryOnNotFound(
			func() (string, error) { calls++; return "loan-1", nil },
			isRace,
			func(d time.Duration) { waited = append(waited, d) },
		)
		require.NoError(t, err)
		assert.Equal(t, "loan-1", got)
		assert.Equal(t, 1, calls)
		assert.Empty(t, waited, "must not wait when the first attempt succeeds")
	})

	t.Run("retries a race error and succeeds once the row appears", func(t *testing.T) {
		calls := 0
		var waited []time.Duration
		got, err := retryOnNotFound(
			func() (string, error) {
				calls++
				if calls < 3 {
					return "", notFound
				}
				return "loan-1", nil
			},
			isRace,
			func(d time.Duration) { waited = append(waited, d) },
		)
		require.NoError(t, err)
		assert.Equal(t, "loan-1", got)
		assert.Equal(t, 3, calls)
		assert.Len(t, waited, 2, "waits between attempts, not after the last one")
	})

	t.Run("gives up after disbursementLookupRetries attempts", func(t *testing.T) {
		calls := 0
		got, err := retryOnNotFound(
			func() (string, error) { calls++; return "", notFound },
			isRace,
			func(time.Duration) {},
		)
		assert.ErrorIs(t, err, notFound)
		assert.Empty(t, got)
		assert.Equal(t, disbursementLookupRetries, calls)
	})

	t.Run("a non-race error returns immediately without retrying", func(t *testing.T) {
		calls := 0
		_, err := retryOnNotFound(
			func() (string, error) { calls++; return "", otherErr },
			isRace,
			func(time.Duration) { t.Fatal("must not wait on a non-race error") },
		)
		assert.ErrorIs(t, err, otherErr)
		assert.Equal(t, 1, calls, "a real failure must not consume the race retry budget")
	})
}

type vaultRepayRepo struct {
	repository.LoanRepository
	claimable    bool
	claimErr     error
	claimAmounts []int64
	fields       []map[string]any
}

func (r *vaultRepayRepo) ClaimVaultRepay(_ context.Context, _ string, amount *int64, _ time.Time) (bool, error) {
	r.claimAmounts = append(r.claimAmounts, *amount)
	return r.claimable, r.claimErr
}

func (r *vaultRepayRepo) UpdateFields(_ context.Context, _ string, fields map[string]any) error {
	r.fields = append(r.fields, fields)
	return nil
}

type vaultRepayStellar struct {
	stellar.Service
	err     error
	amounts []int64
}

func (s *vaultRepayStellar) RepayToVault(_ context.Context, req stellar.RepayRequest) (*stellar.RepayResponse, error) {
	s.amounts = append(s.amounts, req.Amount)
	if s.err != nil {
		return nil, s.err
	}
	return &stellar.RepayResponse{TxHash: "repay-tx", AmountRepaid: req.Amount}, nil
}

type recordingAlerts struct{ subjects []string }

func (a *recordingAlerts) AlertOps(subject, _ string) error {
	a.subjects = append(a.subjects, subject)
	return nil
}

func newVaultRepayAdapter(repo *vaultRepayRepo, st *vaultRepayStellar, alerts *recordingAlerts) *DisbursementStatusAdapter {
	deps := DisbursementAdapterDeps{
		Repo:                  repo,
		StellarSvc:            st,
		Logger:                slog.New(slog.DiscardHandler),
		VaultRepayMaxAttempts: 3,
	}
	if alerts != nil {
		deps.Alerts = alerts
	}
	return NewDisbursementStatusAdapter(deps)
}

func borrowedLoan(attempts int) *models.Loan {
	return &models.Loan{
		ID:                 "loan-1",
		PrincipalAmount:    100_000_000,
		VaultTxHash:        lo.ToPtr("borrow-tx"),
		VaultRepayAttempts: attempts,
	}
}

func TestRepayVault_SuccessRecordsTheHash(t *testing.T) {
	repo := &vaultRepayRepo{claimable: true}
	st := &vaultRepayStellar{}
	a := newVaultRepayAdapter(repo, st, nil)

	require.NoError(t, a.repayVaultIfNeeded(t.Context(), borrowedLoan(0), "fiat_failed", nil))

	assert.Equal(t, []int64{100_000_000}, st.amounts)
	require.Len(t, repo.fields, 1)
	assert.Equal(t, "repay-tx", repo.fields[0]["vault_repay_tx_hash"])
	assert.Equal(t, models.VaultRepayStatusSuccess, repo.fields[0]["vault_repay_status"])
}

func TestRepayVault_AmountPrecedence(t *testing.T) {
	stored := int64(60_000_000)
	override := int64(70_000_000)

	cases := []struct {
		name     string
		stored   *int64
		override *int64
		want     int64
	}{
		{"principal when nothing recorded", nil, nil, 100_000_000},
		{"stored amount over principal", &stored, nil, stored},
		{"override over stored amount", &stored, &override, override},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &vaultRepayRepo{claimable: true}
			st := &vaultRepayStellar{}
			l := borrowedLoan(0)
			l.VaultRepayAmountStroops = tc.stored

			require.NoError(t, newVaultRepayAdapter(repo, st, nil).repayVaultIfNeeded(t.Context(), l, "test", tc.override))
			assert.Equal(t, []int64{tc.want}, st.amounts)
			assert.Equal(t, []int64{tc.want}, repo.claimAmounts)
		})
	}
}

func TestRepayVault_AtCapDefersWithoutClaiming(t *testing.T) {
	repo := &vaultRepayRepo{claimable: true}
	st := &vaultRepayStellar{}

	err := newVaultRepayAdapter(repo, st, nil).repayVaultIfNeeded(t.Context(), borrowedLoan(3), "anchor_refund", nil)

	require.ErrorIs(t, err, contracts.ErrVaultRepayDeferred)
	assert.Empty(t, repo.claimAmounts)
	assert.Empty(t, st.amounts)
}

func TestRepayVault_ReconcilerBypassesTheCap(t *testing.T) {
	repo := &vaultRepayRepo{claimable: true}
	st := &vaultRepayStellar{}
	l := borrowedLoan(7)
	l.VaultRepayAmountStroops = lo.ToPtr(int64(55_000_000))

	require.NoError(t, newVaultRepayAdapter(repo, st, nil).ReconcileVaultRepay(t.Context(), l))
	assert.Equal(t, []int64{55_000_000}, st.amounts, "the reconciler repays the recorded amount")
}

func TestRepayVault_UnclaimableDefers(t *testing.T) {
	repo := &vaultRepayRepo{claimable: false}
	st := &vaultRepayStellar{}

	err := newVaultRepayAdapter(repo, st, nil).repayVaultIfNeeded(t.Context(), borrowedLoan(0), "fiat_failed", nil)

	require.ErrorIs(t, err, contracts.ErrVaultRepayDeferred)
	assert.Empty(t, st.amounts, "a repay held by another caller must not be submitted twice")
}

func TestRepayVault_AlreadyRepaidIsANoop(t *testing.T) {
	repo := &vaultRepayRepo{claimable: true}
	st := &vaultRepayStellar{}
	l := borrowedLoan(0)
	l.VaultRepayTxHash = lo.ToPtr("done")

	require.NoError(t, newVaultRepayAdapter(repo, st, nil).ReconcileVaultRepay(t.Context(), l))
	assert.Empty(t, repo.claimAmounts)
	assert.Empty(t, st.amounts)
}

func TestRepayVault_FailureCountsAndAlertsOnceAtTheCap(t *testing.T) {
	cases := []struct {
		name      string
		attempts  int
		wantAlert bool
	}{
		{"below the cap", 0, false},
		{"reaching the cap", 2, true},
		{"past the cap via the reconciler", 3, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &vaultRepayRepo{claimable: true}
			alerts := &recordingAlerts{}
			a := newVaultRepayAdapter(repo, &vaultRepayStellar{err: errors.New("contract error")}, alerts)

			err := a.ReconcileVaultRepay(t.Context(), borrowedLoan(tc.attempts))

			require.Error(t, err)
			require.NotErrorIs(t, err, contracts.ErrVaultRepayDeferred)
			require.Len(t, repo.fields, 1)
			assert.Equal(t, models.VaultRepayStatusFailed, repo.fields[0]["vault_repay_status"])
			assert.Contains(t, repo.fields[0], "vault_repay_attempts")
			if tc.wantAlert {
				assert.Equal(t, []string{"Vault repay attempts exhausted"}, alerts.subjects)
			} else {
				assert.Empty(t, alerts.subjects)
			}
		})
	}
}

func TestRepayVault_UnconfirmedOutcomeIsParkedUnknown(t *testing.T) {
	for _, cause := range []error{stellar.ErrTransactionTimeout, stellar.ErrUnknownTransactionStatus} {
		t.Run(cause.Error(), func(t *testing.T) {
			repo := &vaultRepayRepo{claimable: true}
			alerts := &recordingAlerts{}
			a := newVaultRepayAdapter(repo, &vaultRepayStellar{err: cause}, alerts)

			require.Error(t, a.repayVaultIfNeeded(t.Context(), borrowedLoan(0), "fiat_failed", nil))

			require.Len(t, repo.fields, 1)
			assert.Equal(t, map[string]any{"vault_repay_status": models.VaultRepayStatusUnknown}, repo.fields[0],
				"an unconfirmed submit must not count as a retryable failure")
			assert.Equal(t, []string{"Vault repay outcome unknown"}, alerts.subjects)
		})
	}
}

func TestVaultRepayColumnsAreUpdatable(t *testing.T) {
	for _, col := range []string{
		"vault_repay_tx_hash", "vault_repay_status", "vault_repay_attempts",
		"vault_repay_amount_stroops", "vault_repay_attempted_at",
	} {
		assert.True(t, repository.IsUpdatableColumn(col), col)
	}
}
