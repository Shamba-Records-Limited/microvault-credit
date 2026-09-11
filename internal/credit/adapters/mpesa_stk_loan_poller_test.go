package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	txmodels "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/mpesa"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
)

type fakeQuerier struct {
	resp *mpesa.ExpressQueryResponse
	err  error

	calls    int
	checkout string
}

func (f *fakeQuerier) ExpressQuery(_ context.Context, checkoutRequestID string, _ uint) (*mpesa.ExpressQueryResponse, error) {
	f.calls++
	f.checkout = checkoutRequestID
	return f.resp, f.err
}

type fakeSTKMpesaRepo struct {
	corerepository.MpesaTransactionRepository
	obs        *txmodels.MpesaTransaction
	lookupErr  error
	confirmErr error
	confirmed  []string
}

func (f *fakeSTKMpesaRepo) GetByCheckoutID(_ context.Context, _ string) (*txmodels.MpesaTransaction, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	return f.obs, nil
}

func (f *fakeSTKMpesaRepo) Confirm(_ context.Context, transID string, _ txmodels.MpesaTransactionConfirmVia, loanID string) error {
	if f.confirmErr != nil {
		return f.confirmErr
	}
	f.confirmed = append(f.confirmed, transID+"|"+loanID)
	return nil
}

type fakeSTKLoanRepo struct {
	repository.LoanRepository
	loan    *models.Loan
	updates []*models.Loan
}

func (f *fakeSTKLoanRepo) GetByID(_ context.Context, _ string) (*models.Loan, error) {
	return f.loan, nil
}

func (f *fakeSTKLoanRepo) Update(_ context.Context, l *models.Loan) error {
	f.updates = append(f.updates, l)
	return nil
}

func newTestSTKDriver(t *testing.T, q *fakeQuerier, m *fakeSTKMpesaRepo, r *fakeSTKLoanRepo, loans *fakeLoanSvc) *MpesaSTKLoanDriver {
	t.Helper()
	return &MpesaSTKLoanDriver{
		client:      q,
		mpesaRepo:   m,
		repo:        r,
		loanSvc:     loans,
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		shortcode:   174379,
		interval:    5 * time.Second,
		maxAttempts: 3,
		now:         func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) },
	}
}

func stkLoan(attempts int) *models.Loan {
	checkout := "ws_CO_stub1"
	return &models.Loan{
		ID:                       "loan-1",
		RepaymentStatus:          models.LoanRepaymentStatusInitiated,
		RepaymentMpesaCheckoutID: &checkout,
		RepaymentSTKAttempts:     attempts,
	}
}

func queryResp(code string) *mpesa.ExpressQueryResponse {
	return &mpesa.ExpressQueryResponse{ResultCode: json.RawMessage(code)}
}

func TestNewMpesaSTKLoanDriver_MissingDependency(t *testing.T) {
	_, err := NewMpesaSTKLoanDriver(MpesaSTKLoanDriverDeps{})
	require.Error(t, err)
}

func TestSTKDrive_SettleConfirmsTheCallbackRow(t *testing.T) {
	q := &fakeQuerier{resp: queryResp("0")}
	m := &fakeSTKMpesaRepo{obs: &txmodels.MpesaTransaction{TransID: "NLJ7RT61SV"}}
	r := &fakeSTKLoanRepo{loan: stkLoan(0)}
	loans := &fakeLoanSvc{}
	d := newTestSTKDriver(t, q, m, r, loans)

	d.Drive(context.Background(), stkLoan(0))

	assert.Equal(t, "ws_CO_stub1", q.checkout)
	assert.Equal(t, []string{"NLJ7RT61SV|loan-1"}, m.confirmed)
	require.Len(t, r.updates, 1)
	assert.Equal(t, models.LoanRepaymentStatusFundsReceived, r.updates[0].RepaymentStatus)
	require.NotNil(t, r.updates[0].RepaymentMpesaTransID)
	assert.Equal(t, "NLJ7RT61SV", *r.updates[0].RepaymentMpesaTransID)
	assert.Nil(t, r.updates[0].RepaymentNextPollAt)
}

func TestSTKDrive_SettleWithoutACallbackRow(t *testing.T) {
	q := &fakeQuerier{resp: queryResp("0")}
	m := &fakeSTKMpesaRepo{lookupErr: corerepository.ErrMpesaNotFound}
	r := &fakeSTKLoanRepo{loan: stkLoan(0)}
	loans := &fakeLoanSvc{}
	d := newTestSTKDriver(t, q, m, r, loans)

	d.Drive(context.Background(), stkLoan(0))

	assert.Empty(t, m.confirmed)
	require.Len(t, r.updates, 1)
	assert.Equal(t, models.LoanRepaymentStatusFundsReceived, r.updates[0].RepaymentStatus)
	assert.Nil(t, r.updates[0].RepaymentMpesaTransID)
}

func TestSTKDrive_SettleGuardsAgainstAConcurrentSettlement(t *testing.T) {
	q := &fakeQuerier{resp: queryResp("0")}
	m := &fakeSTKMpesaRepo{obs: &txmodels.MpesaTransaction{TransID: "NLJ7RT61SV"}}
	settled := stkLoan(0)
	settled.RepaymentStatus = models.LoanRepaymentStatusSettled
	r := &fakeSTKLoanRepo{loan: settled}
	loans := &fakeLoanSvc{}
	d := newTestSTKDriver(t, q, m, r, loans)

	d.Drive(context.Background(), stkLoan(0))

	assert.Empty(t, r.updates, "an already-settled loan is left alone")
}

func TestSTKDrive_ConfirmFailureStillSettles(t *testing.T) {
	q := &fakeQuerier{resp: queryResp("0")}
	m := &fakeSTKMpesaRepo{
		obs:        &txmodels.MpesaTransaction{TransID: "NLJ7RT61SV"},
		confirmErr: errors.New("db down"),
	}
	r := &fakeSTKLoanRepo{loan: stkLoan(0)}
	loans := &fakeLoanSvc{}
	d := newTestSTKDriver(t, q, m, r, loans)

	d.Drive(context.Background(), stkLoan(0))

	require.Len(t, r.updates, 1)
	assert.Equal(t, models.LoanRepaymentStatusFundsReceived, r.updates[0].RepaymentStatus)
	assert.Nil(t, r.updates[0].RepaymentMpesaTransID)
}

func TestSTKDrive_RetryableAnswerParksWithTheAttempt(t *testing.T) {
	q := &fakeQuerier{resp: queryResp("1032")}
	m := &fakeSTKMpesaRepo{}
	r := &fakeSTKLoanRepo{loan: stkLoan(0)}
	loans := &fakeLoanSvc{}
	d := newTestSTKDriver(t, q, m, r, loans)

	d.Drive(context.Background(), stkLoan(0))

	assert.Empty(t, r.updates)
	require.Len(t, loans.updates, 1)
	require.NotNil(t, loans.updates[0].RepaymentSTKAttempts)
	assert.Equal(t, 1, *loans.updates[0].RepaymentSTKAttempts)
	require.NotNil(t, loans.updates[0].RepaymentNextPollAt)
	assert.Equal(t, time.Date(2026, 9, 7, 12, 0, 5, 0, time.UTC), *loans.updates[0].RepaymentNextPollAt)
}

func TestSTKDrive_LastAttemptExpires(t *testing.T) {
	q := &fakeQuerier{resp: queryResp("1032")}
	m := &fakeSTKMpesaRepo{}
	r := &fakeSTKLoanRepo{loan: stkLoan(2)}
	loans := &fakeLoanSvc{}
	d := newTestSTKDriver(t, q, m, r, loans)

	d.Drive(context.Background(), stkLoan(2))

	assert.Empty(t, loans.updates)
	require.Len(t, r.updates, 1)
	assert.Equal(t, models.LoanRepaymentStatusExpired, r.updates[0].RepaymentStatus)
	assert.Nil(t, r.updates[0].RepaymentNextPollAt)
}

// TestSTKDrive_DocumentedOperationalFailureExpiresImmediately covers a
// result code mpesa.expressOutcomes actually documents as non-retryable
// (2028 — Daraja's own "operator does not exist" family). This is the one
// class of failure that still closes on the first poll: we know for
// certain it won't resolve differently on a retry.
func TestSTKDrive_DocumentedOperationalFailureExpiresImmediately(t *testing.T) {
	q := &fakeQuerier{resp: queryResp("2028")}
	m := &fakeSTKMpesaRepo{}
	r := &fakeSTKLoanRepo{loan: stkLoan(0)}
	loans := &fakeLoanSvc{}
	d := newTestSTKDriver(t, q, m, r, loans)

	d.Drive(context.Background(), stkLoan(0))

	assert.Empty(t, loans.updates)
	require.Len(t, r.updates, 1)
	assert.Equal(t, models.LoanRepaymentStatusExpired, r.updates[0].RepaymentStatus)
}

// TestSTKDrive_UndocumentedFailureGetsARetryBudget covers a result code not
// in mpesa.expressOutcomes at all (9999) — see
// yellowcard-offramp-webhook-race-2026-09-10.md §3 in the knowledge vault
// for why "undocumented" stopped meaning "expire with zero retries":
// mpesa.ExpressOutcomeFor now gives an unknown code the same bounded retry
// budget a known-transient one gets, rather than assuming it is permanent.
func TestSTKDrive_UndocumentedFailureGetsARetryBudget(t *testing.T) {
	q := &fakeQuerier{resp: queryResp("9999")}
	m := &fakeSTKMpesaRepo{}
	r := &fakeSTKLoanRepo{loan: stkLoan(0)}
	loans := &fakeLoanSvc{}
	d := newTestSTKDriver(t, q, m, r, loans)

	d.Drive(context.Background(), stkLoan(0))

	assert.Empty(t, r.updates, "must not close on the first poll of an unknown code")
	require.Len(t, loans.updates, 1)
	require.NotNil(t, loans.updates[0].RepaymentSTKAttempts)
	assert.Equal(t, 1, *loans.updates[0].RepaymentSTKAttempts)
	require.NotNil(t, loans.updates[0].RepaymentNextPollAt)
}

// TestSTKDrive_UndocumentedFailureExpiresAfterMaxAttempts confirms the
// retry budget from the test above is genuinely bounded — an undocumented
// code that keeps recurring still closes once maxAttempts is exhausted,
// exactly like a documented transient one would.
func TestSTKDrive_UndocumentedFailureExpiresAfterMaxAttempts(t *testing.T) {
	q := &fakeQuerier{resp: queryResp("9999")}
	m := &fakeSTKMpesaRepo{}
	r := &fakeSTKLoanRepo{loan: stkLoan(2)} // newTestSTKDriver's maxAttempts is 3
	loans := &fakeLoanSvc{}
	d := newTestSTKDriver(t, q, m, r, loans)

	d.Drive(context.Background(), stkLoan(2))

	assert.Empty(t, loans.updates)
	require.Len(t, r.updates, 1)
	assert.Equal(t, models.LoanRepaymentStatusExpired, r.updates[0].RepaymentStatus)
}

func TestSTKDrive_TransportErrorParksWithoutCounting(t *testing.T) {
	q := &fakeQuerier{err: errors.New("connection reset")}
	m := &fakeSTKMpesaRepo{}
	r := &fakeSTKLoanRepo{loan: stkLoan(1)}
	loans := &fakeLoanSvc{}
	d := newTestSTKDriver(t, q, m, r, loans)

	d.Drive(context.Background(), stkLoan(1))

	assert.Empty(t, r.updates)
	require.Len(t, loans.updates, 1)
	require.NotNil(t, loans.updates[0].RepaymentSTKAttempts)
	assert.Equal(t, 1, *loans.updates[0].RepaymentSTKAttempts, "attempts only move on an answer from Daraja")
	require.NotNil(t, loans.updates[0].RepaymentNextPollAt)
}

func TestSTKDrive_MissingCheckoutIDQueriesNothing(t *testing.T) {
	q := &fakeQuerier{}
	m := &fakeSTKMpesaRepo{}
	r := &fakeSTKLoanRepo{}
	loans := &fakeLoanSvc{}
	d := newTestSTKDriver(t, q, m, r, loans)

	d.Drive(context.Background(), &models.Loan{ID: "loan-1"})

	assert.Zero(t, q.calls)
	assert.Empty(t, r.updates)
	assert.Empty(t, loans.updates)
}
