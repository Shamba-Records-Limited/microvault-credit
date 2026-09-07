package loan

import (
	"context"
	"errors"
	"testing"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/loanref"
)

// fakeRepo captures every Create attempt. The service retries with the same
// loan pointer, so attempts are recorded by value at call time.
type fakeRepo struct {
	repository.LoanRepository
	conflicts int
	refs      []string
	fail      error
}

func (f *fakeRepo) Create(_ context.Context, loan *models.Loan) error {
	if loan.LoanReference == nil {
		f.refs = append(f.refs, "")
	} else {
		f.refs = append(f.refs, *loan.LoanReference)
	}
	if f.conflicts > 0 {
		f.conflicts--
		return repository.ErrLoanReferenceConflict
	}
	return f.fail
}

func validRequest() CreateLoanRequest {
	return CreateLoanRequest{
		UserID:            "user-1",
		AccountID:         "acct-1",
		PrincipalAmount:   10_000,
		PrincipalAsset:    "KES",
		VaultAPRBps:       1_000,
		DurationDays:      30,
		RepaymentSchedule: "monthly",
	}
}

func TestCreate_UsesConfiguredPrefix(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, "AA")

	resp, err := svc.Create(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ref := *resp.LoanReference
	if ref[:2] != "AA" {
		t.Errorf("reference %q does not carry the configured prefix", ref)
	}
	if !loanref.Validate("AA", ref) {
		t.Errorf("reference %q fails validation under its own prefix", ref)
	}
}

// A unique conflict mints a fresh reference and retries rather than failing.
func TestCreate_RetriesOnReferenceConflict(t *testing.T) {
	repo := &fakeRepo{conflicts: 2}
	svc := NewService(repo, loanref.DefaultPrefix)

	if _, err := svc.Create(context.Background(), validRequest()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(repo.refs) != 3 {
		t.Fatalf("attempts = %d, want 3 (initial + 2 retries)", len(repo.refs))
	}

	seen := map[string]bool{}
	for _, ref := range repo.refs {
		if ref == "" {
			t.Fatal("an attempt went out with no reference")
		}
		if seen[ref] {
			t.Errorf("retry reused reference %q", ref)
		}
		seen[ref] = true
	}
}

// Persistent conflicts exhaust the bounded retry rather than looping.
func TestCreate_ExhaustsRetriesOnPersistentConflict(t *testing.T) {
	repo := &fakeRepo{conflicts: 99}
	svc := NewService(repo, loanref.DefaultPrefix)

	_, err := svc.Create(context.Background(), validRequest())
	if !errors.Is(err, repository.ErrLoanReferenceConflict) {
		t.Errorf("err = %v, want ErrLoanReferenceConflict", err)
	}
	if len(repo.refs) != referenceAttempts {
		t.Errorf("attempts = %d, want %d", len(repo.refs), referenceAttempts)
	}
}

// A non-conflict failure is not retried.
func TestCreate_DoesNotRetryOtherFailures(t *testing.T) {
	repo := &fakeRepo{fail: repository.ErrFailedToCreateLoan}
	svc := NewService(repo, loanref.DefaultPrefix)

	if _, err := svc.Create(context.Background(), validRequest()); err == nil {
		t.Fatal("expected the repository failure to surface")
	}
	if len(repo.refs) != 1 {
		t.Errorf("attempts = %d, want 1", len(repo.refs))
	}
}
