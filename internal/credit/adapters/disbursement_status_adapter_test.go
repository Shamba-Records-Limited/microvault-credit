package adapters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	stellarrpc "github.com/Shamba-Records-Limited/microvault/pkg/stellar/rpc"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"

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

type settleCall struct {
	guard  repository.VaultRepayGuard
	fields map[string]any
}

type vaultRepayRepo struct {
	repository.LoanRepository
	claimable    bool
	claimErr     error
	claimAmounts []int64
	settleFail   map[int]bool // call index -> report not settled
	settles      []settleCall
	fields       []map[string]any
}

func (r *vaultRepayRepo) ClaimVaultRepay(_ context.Context, _ string, amount *int64, _ time.Time) (bool, error) {
	r.claimAmounts = append(r.claimAmounts, *amount)
	return r.claimable, r.claimErr
}

func (r *vaultRepayRepo) SettleVaultRepay(_ context.Context, _ string, guard repository.VaultRepayGuard, fields map[string]any) (bool, error) {
	r.settles = append(r.settles, settleCall{guard: guard, fields: fields})
	return !r.settleFail[len(r.settles)-1], nil
}

func (r *vaultRepayRepo) UpdateFields(_ context.Context, _ string, fields map[string]any) error {
	r.fields = append(r.fields, fields)
	return nil
}

func (r *vaultRepayRepo) last() settleCall { return r.settles[len(r.settles)-1] }

var testValidUntil = time.Date(2026, 10, 2, 12, 5, 0, 0, time.UTC)

// vaultRepayStellar signs before it fails unless failBeforeSigning is set,
// matching the real submit path.
type vaultRepayStellar struct {
	stellar.Service
	err               error
	failBeforeSigning bool
	amounts           []int64
}

func (s *vaultRepayStellar) RepayToVault(ctx context.Context, req stellar.RepayRequest) (*stellar.RepayResponse, error) {
	s.amounts = append(s.amounts, req.Amount)
	if s.err != nil && s.failBeforeSigning {
		return nil, s.err
	}
	if req.OnSigned != nil {
		if err := req.OnSigned(ctx, "repay-tx", testValidUntil); err != nil {
			return nil, err
		}
	}
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

type fakeTxResolver struct {
	res stellarrpc.TxResolution
	err error
}

func (f fakeTxResolver) ResolveSubmitted(context.Context, string, time.Time, time.Time) (stellarrpc.TxResolution, error) {
	return f.res, f.err
}

type countingTxnSvc struct {
	transaction.Service
	created int
}

func (c *countingTxnSvc) Create(context.Context, transaction.CreateTransactionRequest) (*transaction.TransactionResponse, error) {
	c.created++
	return nil, nil
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

func TestRepayVault_RecordsTheSignedHashThenSettlesOnIt(t *testing.T) {
	repo := &vaultRepayRepo{claimable: true}
	a := newVaultRepayAdapter(repo, &vaultRepayStellar{}, nil)

	require.NoError(t, a.repayVaultIfNeeded(t.Context(), borrowedLoan(0), "fiat_failed", nil))

	require.Len(t, repo.settles, 2)
	assert.Equal(t, repository.VaultRepayGuard{Status: models.VaultRepayStatusPending}, repo.settles[0].guard)
	assert.Equal(t, map[string]any{"vault_repay_pending_tx_hash": "repay-tx", "vault_repay_tx_expires_at": testValidUntil},
		repo.settles[0].fields, "the hash is recorded before submission")
	assert.Equal(t, repository.VaultRepayGuard{PendingTxHash: "repay-tx"}, repo.settles[1].guard)
	assert.Equal(t, vaultRepaySuccessFields("repay-tx"), repo.settles[1].fields)
}

func TestRepayVault_LostClaimAbortsBeforeSubmission(t *testing.T) {
	repo := &vaultRepayRepo{claimable: true, settleFail: map[int]bool{0: true}}
	st := &vaultRepayStellar{}

	err := newVaultRepayAdapter(repo, st, nil).repayVaultIfNeeded(t.Context(), borrowedLoan(0), "fiat_failed", nil)

	require.Error(t, err)
	assert.Equal(t, vaultRepayFailedFields()["vault_repay_status"], repo.last().fields["vault_repay_status"])
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
			assert.Equal(t, repository.VaultRepayGuard{Status: models.VaultRepayStatusPending}, repo.last().guard)
			assert.Equal(t, models.VaultRepayStatusFailed, repo.last().fields["vault_repay_status"])
			assert.Contains(t, repo.last().fields, "vault_repay_attempts")
			if tc.wantAlert {
				assert.Equal(t, []string{"Vault repay attempts exhausted"}, alerts.subjects)
			} else {
				assert.Empty(t, alerts.subjects)
			}
		})
	}
}

func TestRepayVault_UnconfirmedOutcomeKeepsTheHashForTheReconciler(t *testing.T) {
	for _, cause := range []error{stellar.ErrTransactionTimeout, stellar.ErrUnknownTransactionStatus, stellar.ErrSubmissionUnconfirmed} {
		t.Run(cause.Error(), func(t *testing.T) {
			repo := &vaultRepayRepo{claimable: true}
			alerts := &recordingAlerts{}
			a := newVaultRepayAdapter(repo, &vaultRepayStellar{err: cause}, alerts)

			require.Error(t, a.repayVaultIfNeeded(t.Context(), borrowedLoan(0), "fiat_failed", nil))

			assert.Equal(t, map[string]any{"vault_repay_status": models.VaultRepayStatusUnknown}, repo.last().fields,
				"an unconfirmed submit must not count as an attempt or drop its hash")
			assert.Empty(t, alerts.subjects, "the reconciler settles it; no human needed")
		})
	}
}

func TestRepayVault_RefusedSubmissionSpendsAnAttemptAndDropsTheHash(t *testing.T) {
	for _, cause := range []error{stellar.ErrTransactionRejected, stellar.ErrStellarCoreOverloaded} {
		t.Run(cause.Error(), func(t *testing.T) {
			repo := &vaultRepayRepo{claimable: true}
			a := newVaultRepayAdapter(repo, &vaultRepayStellar{err: cause}, nil)

			require.Error(t, a.repayVaultIfNeeded(t.Context(), borrowedLoan(0), "fiat_failed", nil))

			assert.Equal(t, vaultRepayFailedFields(), repo.last().fields,
				"stellar-core never admitted it, so there is nothing for the ledger to settle")
		})
	}
}

func TestRepayVault_UnconfirmedWithoutAHashAlerts(t *testing.T) {
	repo := &vaultRepayRepo{claimable: true}
	alerts := &recordingAlerts{}
	a := newVaultRepayAdapter(repo, &vaultRepayStellar{err: stellar.ErrTransactionTimeout, failBeforeSigning: true}, alerts)

	require.Error(t, a.repayVaultIfNeeded(t.Context(), borrowedLoan(0), "fiat_failed", nil))

	assert.Equal(t, []string{"Vault repay outcome unknown"}, alerts.subjects)
}

func TestSettleVaultRepaySuccess_RecordsTheAuditTransactionOnce(t *testing.T) {
	for _, settled := range []bool{true, false} {
		t.Run(fmt.Sprintf("settled=%v", settled), func(t *testing.T) {
			repo := &vaultRepayRepo{settleFail: map[int]bool{0: !settled}}
			txns := &countingTxnSvc{}
			a := newVaultRepayAdapter(repo, &vaultRepayStellar{}, nil)
			a.txnSvc = txns

			a.settleVaultRepaySuccess(t.Context(), borrowedLoan(0), repository.VaultRepayGuard{PendingTxHash: "repay-tx"},
				&stellar.RepayResponse{TxHash: "repay-tx", AmountRepaid: 1}, "test")

			assert.Equal(t, lo.Ternary(settled, 1, 0), txns.created)
		})
	}
}

func unconfirmedLoan() *models.Loan {
	l := borrowedLoan(1)
	l.VaultRepayStatus = lo.ToPtr(models.VaultRepayStatusUnknown)
	l.VaultRepayPendingTxHash = lo.ToPtr("repay-tx")
	l.VaultRepayAttemptedAt = lo.ToPtr(testValidUntil.Add(-5 * time.Minute))
	l.VaultRepayTxExpiresAt = lo.ToPtr(testValidUntil)
	l.VaultRepayAmountStroops = lo.ToPtr(int64(40_000_000))
	return l
}

func TestResolveVaultRepay(t *testing.T) {
	hashGuard := repository.VaultRepayGuard{PendingTxHash: "repay-tx"}
	cases := []struct {
		name       string
		resolver   fakeTxResolver
		wantFields map[string]any
		wantAlert  []string
		wantErr    bool
	}{
		{
			name:       "landed is recorded as success",
			resolver:   fakeTxResolver{res: stellarrpc.TxResolution{Outcome: stellarrpc.TxSucceeded, Ledger: 9}},
			wantFields: vaultRepaySuccessFields("repay-tx"),
		},
		{
			name:       "failed on ledger spends an attempt",
			resolver:   fakeTxResolver{res: stellarrpc.TxResolution{Outcome: stellarrpc.TxFailed}},
			wantFields: vaultRepayFailedFields(),
		},
		{
			name:       "never landed spends an attempt",
			resolver:   fakeTxResolver{res: stellarrpc.TxResolution{Outcome: stellarrpc.TxNeverLanded}},
			wantFields: vaultRepayFailedFields(),
		},
		{
			name:     "outside retention is parked for an operator",
			resolver: fakeTxResolver{res: stellarrpc.TxResolution{Outcome: stellarrpc.TxOutsideRetention}},
			wantFields: map[string]any{
				"vault_repay_status":          models.VaultRepayStatusUnknown,
				"vault_repay_pending_tx_hash": nil,
				"vault_repay_tx_expires_at":   nil,
			},
			wantAlert: []string{"Vault repay outcome unresolvable"},
		},
		{
			name:     "unresolved writes nothing",
			resolver: fakeTxResolver{res: stellarrpc.TxResolution{Outcome: stellarrpc.TxUnresolved}},
		},
		{
			name:     "lookup error writes nothing",
			resolver: fakeTxResolver{err: errors.New("rpc down")},
			wantErr:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &vaultRepayRepo{}
			alerts := &recordingAlerts{}
			a := newVaultRepayAdapter(repo, &vaultRepayStellar{}, alerts)
			a.txResolver = tc.resolver

			err := a.ResolveVaultRepay(t.Context(), unconfirmedLoan())

			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tc.wantFields == nil {
				assert.Empty(t, repo.settles)
			} else {
				require.Len(t, repo.settles, 1)
				assert.Equal(t, hashGuard, repo.settles[0].guard, "resolution only applies to the transaction it looked up")
				assert.Equal(t, tc.wantFields, repo.settles[0].fields)
			}
			assert.Equal(t, tc.wantAlert, alerts.subjects)
		})
	}
}

func TestVaultRepayColumnsAreUpdatable(t *testing.T) {
	for _, col := range []string{
		"vault_repay_tx_hash", "vault_repay_status", "vault_repay_attempts",
		"vault_repay_amount_stroops", "vault_repay_attempted_at",
		"vault_repay_pending_tx_hash", "vault_repay_tx_expires_at",
	} {
		assert.True(t, repository.IsUpdatableColumn(col), col)
	}
}
