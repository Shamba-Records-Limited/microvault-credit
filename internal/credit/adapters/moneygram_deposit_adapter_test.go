package adapters

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/payment/stellaranchor"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
)

// stubStellarService satisfies the constructor's required-dependency check.
// None of these tests reach the chain.
type stubStellarService struct{ stellar.Service }

func newTestDepositAdapter(t *testing.T, repo *fakeLoanRepo) (*MoneyGramDepositAdapter, *fakeLoanSvc) {
	t.Helper()
	loans := &fakeLoanSvc{}
	a, err := NewMoneyGramDepositAdapter(DepositAdapterDeps{
		Repo:       repo,
		LoanSvc:    loans,
		StellarSvc: stubStellarService{},
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(t, err)
	return a, loans
}

const sandboxMoreInfoURL = "https://extramps.moneygram.com/transaction-status?transaction_id=4a93bfcf&token=eyJhbGciOiJIUzI1NiJ9"

// MoneyGram populates more_info_url from the first poll while
// external_transaction_id stays empty until after the borrower has paid. The
// page is therefore the only thing they can act on, and it has to be stored
// before the notifier can send it.
func TestRecordDepositUpdate_PersistsTheTransactionPage(t *testing.T) {
	repo := &fakeLoanRepo{loan: &models.Loan{}}
	a, loans := newTestDepositAdapter(t, repo)

	err := a.RecordDepositUpdate(context.Background(), "loan-1", &stellaranchor.Transaction{
		Status:      stellaranchor.StatusPendingUserTransferStart,
		MoreInfoURL: sandboxMoreInfoURL,
	})

	require.NoError(t, err)
	require.Len(t, loans.updates, 1)
	require.NotNil(t, loans.updates[0].RampMoreInfoURL)
	assert.Equal(t, sandboxMoreInfoURL, *loans.updates[0].RampMoreInfoURL)
	require.NotNil(t, loans.updates[0].RampMoreInfoShortCode)
	assert.NotEmpty(t, *loans.updates[0].RampMoreInfoShortCode)
}

// A second code would orphan the first, which by then is sitting in an SMS on
// the borrower's handset pointing at a /r/{code} that no longer resolves.
func TestRecordDepositUpdate_DoesNotMintASecondShortCode(t *testing.T) {
	existing := "Xk9f2aQ7ab"
	url := sandboxMoreInfoURL
	repo := &fakeLoanRepo{loan: &models.Loan{
		RampMoreInfoURL:       &url,
		RampMoreInfoShortCode: &existing,
	}}
	a, loans := newTestDepositAdapter(t, repo)

	err := a.RecordDepositUpdate(context.Background(), "loan-1", &stellaranchor.Transaction{
		Status:      stellaranchor.StatusPendingUserTransferStart,
		MoreInfoURL: sandboxMoreInfoURL,
	})

	require.NoError(t, err)
	assert.Empty(t, loans.updates, "nothing changed, so nothing is written")
}

func TestRecordDepositUpdate_DeadlineAndPageInOneWrite(t *testing.T) {
	repo := &fakeLoanRepo{loan: &models.Loan{}}
	a, loans := newTestDepositAdapter(t, repo)

	err := a.RecordDepositUpdate(context.Background(), "loan-1", &stellaranchor.Transaction{
		Status:               stellaranchor.StatusPendingUserTransferStart,
		UserActionRequiredBy: "2026-08-27T04:45:25Z",
		MoreInfoURL:          sandboxMoreInfoURL,
	})

	require.NoError(t, err)
	require.Len(t, loans.updates, 1, "one poll is one write")
	require.NotNil(t, loans.updates[0].RepaymentExpiresAt)
	assert.Equal(t, "2026-08-27T04:45:25Z", loans.updates[0].RepaymentExpiresAt.Format(time.RFC3339))
	assert.NotNil(t, loans.updates[0].RampMoreInfoURL)
}

func TestRecordDepositUpdate_NothingToRecord(t *testing.T) {
	repo := &fakeLoanRepo{loan: &models.Loan{}}
	a, loans := newTestDepositAdapter(t, repo)

	err := a.RecordDepositUpdate(context.Background(), "loan-1", &stellaranchor.Transaction{
		Status: stellaranchor.StatusIncomplete,
	})

	require.NoError(t, err)
	assert.Empty(t, loans.updates)
}

// The deadline is rewritten whenever MoneyGram moves it, because the last
// value seen is the one to act on.
func TestRecordDepositUpdate_DeadlineIsRewrittenWhenItMoves(t *testing.T) {
	was := time.Date(2026, 8, 27, 4, 45, 25, 0, time.UTC)
	repo := &fakeLoanRepo{loan: &models.Loan{RepaymentExpiresAt: &was}}
	a, loans := newTestDepositAdapter(t, repo)

	require.NoError(t, a.RecordDepositUpdate(context.Background(), "loan-1", &stellaranchor.Transaction{
		UserActionRequiredBy: was.Format(time.RFC3339),
	}))
	assert.Empty(t, loans.updates, "an unchanged deadline costs nothing")

	require.NoError(t, a.RecordDepositUpdate(context.Background(), "loan-1", &stellaranchor.Transaction{
		UserActionRequiredBy: was.Add(2 * time.Hour).Format(time.RFC3339),
	}))
	require.Len(t, loans.updates, 1)
	require.NotNil(t, loans.updates[0].RepaymentExpiresAt)
}
