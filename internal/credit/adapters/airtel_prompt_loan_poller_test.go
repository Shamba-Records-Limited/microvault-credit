package adapters

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coremodels "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/airtel"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
)

type fakeAirtelEnquirer struct {
	status  string
	receipt string
	err     error
	asked   []string
}

func (f *fakeAirtelEnquirer) Enquiry(_ context.Context, transactionID string) (*airtel.EnquiryResponse, error) {
	f.asked = append(f.asked, transactionID)
	if f.err != nil {
		return nil, f.err
	}
	resp := &airtel.EnquiryResponse{}
	resp.Data.Transaction.ID = transactionID
	resp.Data.Transaction.Status = f.status
	resp.Data.Transaction.AirtelMoneyID = f.receipt
	return resp, nil
}

type fakeAirtelCoreRepo struct {
	corerepository.AirtelTransactionRepository
	row       *coremodels.AirtelTransaction
	getErr    error
	confirmed []string
	receipts  []string
}

func (f *fakeAirtelCoreRepo) GetByPartnerID(_ context.Context, _ string) (*coremodels.AirtelTransaction, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.row, nil
}

func (f *fakeAirtelCoreRepo) Confirm(_ context.Context, partnerTxnID string, _ coremodels.AirtelTransactionConfirmVia, _, airtelMoneyID, _ string) error {
	f.confirmed = append(f.confirmed, partnerTxnID)
	f.receipts = append(f.receipts, airtelMoneyID)
	return nil
}

func newTestAirtelDriver(q *fakeAirtelEnquirer, core *fakeAirtelCoreRepo, r *fakeSTKLoanRepo, loans *fakeLoanSvc, n *fakeSTKNotifier) *AirtelPromptLoanDriver {
	return &AirtelPromptLoanDriver{
		client:      q,
		airtelRepo:  core,
		repo:        r,
		loanSvc:     loans,
		notifier:    n,
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		interval:    time.Minute,
		maxAttempts: 3,
		now:         func() time.Time { return time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC) },
	}
}

func airtelPromptLoan(attempts int) *models.Loan {
	return &models.Loan{
		ID:                      "loan-1",
		RepaymentStatus:         models.LoanRepaymentStatusInitiated,
		RepaymentAirtelTxnID:    lo.ToPtr("MV12345678-1758283200"),
		RepaymentAirtelAttempts: attempts,
		RepaymentPayoffStroops:  lo.ToPtr(int64(38_000_000)),
	}
}

func stagedRow() *coremodels.AirtelTransaction {
	return &coremodels.AirtelTransaction{
		PartnerTxnID: "MV12345678-1758283200",
		AmountMinor:  50_000,
	}
}

func TestAirtelDriver_SettlesOnSuccess(t *testing.T) {
	q := &fakeAirtelEnquirer{status: "TS", receipt: "AM000000001"}
	core := &fakeAirtelCoreRepo{row: stagedRow()}
	repo := &fakeSTKLoanRepo{loan: airtelPromptLoan(0)}
	notifier := &fakeSTKNotifier{}

	newTestAirtelDriver(q, core, repo, &fakeLoanSvc{}, notifier).
		Drive(context.Background(), airtelPromptLoan(0))

	require.Len(t, repo.updates, 1)
	settled := repo.updates[0]
	assert.Equal(t, models.LoanRepaymentStatusFundsReceived, settled.RepaymentStatus)
	require.NotNil(t, settled.RepaymentAirtelMoneyID)
	assert.Equal(t, "AM000000001", *settled.RepaymentAirtelMoneyID)
	assert.Nil(t, settled.RepaymentNextPollAt, "a settled loan must leave the due set")

	// The staging row is confirmed against the enquiry, carrying the receipt.
	assert.Equal(t, []string{"MV12345678-1758283200"}, core.confirmed)
	assert.Equal(t, []string{"AM000000001"}, core.receipts)

	// The borrower is told, with the amount the staging row observed.
	assert.Equal(t, []string{"loan-1"}, notifier.notified)
	assert.Equal(t, []int64{500}, notifier.amounts)
}

// A payment whose callback never arrived still settles: the enquiry is
// Airtel's own word, and reconciliation backfills the staging row.
func TestAirtelDriver_SettlesWithoutAStagedCallback(t *testing.T) {
	q := &fakeAirtelEnquirer{status: "TS", receipt: "AM000000002"}
	core := &fakeAirtelCoreRepo{getErr: corerepository.ErrAirtelNotFound}
	repo := &fakeSTKLoanRepo{loan: airtelPromptLoan(0)}

	newTestAirtelDriver(q, core, repo, &fakeLoanSvc{}, &fakeSTKNotifier{}).
		Drive(context.Background(), airtelPromptLoan(0))

	require.Len(t, repo.updates, 1)
	assert.Equal(t, models.LoanRepaymentStatusFundsReceived, repo.updates[0].RepaymentStatus)
	assert.Empty(t, core.confirmed, "there was no row to confirm")
}

// TIP and TA are not terminal: the payer may still be entering a PIN.
func TestAirtelDriver_ReschedulesWhileInProgress(t *testing.T) {
	for _, status := range []string{"TIP", "TA"} {
		t.Run(status, func(t *testing.T) {
			q := &fakeAirtelEnquirer{status: status}
			repo := &fakeSTKLoanRepo{loan: airtelPromptLoan(0)}
			loans := &fakeLoanSvc{}

			newTestAirtelDriver(q, &fakeAirtelCoreRepo{row: stagedRow()}, repo, loans, &fakeSTKNotifier{}).
				Drive(context.Background(), airtelPromptLoan(0))

			assert.Empty(t, repo.updates, "a pending enquiry must not settle the loan")
			require.Len(t, loans.updates, 1)
			require.NotNil(t, loans.updates[0].RepaymentAirtelAttempts)
			assert.Equal(t, 1, *loans.updates[0].RepaymentAirtelAttempts)
			require.NotNil(t, loans.updates[0].RepaymentNextPollAt)
		})
	}
}

func TestAirtelDriver_ExpiresAtTheAttemptCeiling(t *testing.T) {
	q := &fakeAirtelEnquirer{status: "TIP"}
	repo := &fakeSTKLoanRepo{loan: airtelPromptLoan(2)}

	newTestAirtelDriver(q, &fakeAirtelCoreRepo{row: stagedRow()}, repo, &fakeLoanSvc{}, &fakeSTKNotifier{}).
		Drive(context.Background(), airtelPromptLoan(2))

	require.Len(t, repo.updates, 1)
	assert.Equal(t, models.LoanRepaymentStatusExpired, repo.updates[0].RepaymentStatus)
	assert.Nil(t, repo.updates[0].RepaymentNextPollAt)
}

// TF and TE from an enquiry are terminal, unlike the same values on a
// callback.
func TestAirtelDriver_ClosesOnTerminalFailure(t *testing.T) {
	for _, status := range []string{"TF", "TE"} {
		t.Run(status, func(t *testing.T) {
			q := &fakeAirtelEnquirer{status: status}
			repo := &fakeSTKLoanRepo{loan: airtelPromptLoan(0)}

			newTestAirtelDriver(q, &fakeAirtelCoreRepo{row: stagedRow()}, repo, &fakeLoanSvc{}, &fakeSTKNotifier{}).
				Drive(context.Background(), airtelPromptLoan(0))

			require.Len(t, repo.updates, 1)
			assert.Equal(t, models.LoanRepaymentStatusExpired, repo.updates[0].RepaymentStatus)
		})
	}
}

// A transport error means "not yet", never a failure, and must not spend an
// attempt: the counter moves only on an answer from Airtel.
func TestAirtelDriver_TransportErrorDoesNotSpendAnAttempt(t *testing.T) {
	q := &fakeAirtelEnquirer{err: errors.New("gateway down")}
	repo := &fakeSTKLoanRepo{loan: airtelPromptLoan(0)}
	loans := &fakeLoanSvc{}

	newTestAirtelDriver(q, &fakeAirtelCoreRepo{row: stagedRow()}, repo, loans, &fakeSTKNotifier{}).
		Drive(context.Background(), airtelPromptLoan(0))

	assert.Empty(t, repo.updates)
	require.Len(t, loans.updates, 1)
	require.NotNil(t, loans.updates[0].RepaymentAirtelAttempts)
	assert.Equal(t, 0, *loans.updates[0].RepaymentAirtelAttempts,
		"a transport failure is not an answer and must not count")
}

func TestAirtelDriver_RequiresATransactionID(t *testing.T) {
	q := &fakeAirtelEnquirer{status: "TS"}
	repo := &fakeSTKLoanRepo{}

	newTestAirtelDriver(q, &fakeAirtelCoreRepo{}, repo, &fakeLoanSvc{}, &fakeSTKNotifier{}).
		Drive(context.Background(), &models.Loan{ID: "loan-1"})

	assert.Empty(t, q.asked, "nothing should be asked without an id")
	assert.Empty(t, repo.updates)
}

// A loan another path already moved off initiated must not be settled twice.
func TestAirtelDriver_DoesNotResettle(t *testing.T) {
	q := &fakeAirtelEnquirer{status: "TS", receipt: "AM000000003"}
	already := airtelPromptLoan(0)
	already.RepaymentStatus = models.LoanRepaymentStatusFundsReceived
	repo := &fakeSTKLoanRepo{loan: already}

	newTestAirtelDriver(q, &fakeAirtelCoreRepo{row: stagedRow()}, repo, &fakeLoanSvc{}, &fakeSTKNotifier{}).
		Drive(context.Background(), airtelPromptLoan(0))

	assert.Empty(t, repo.updates, "an already-settled loan must not be written again")
}
