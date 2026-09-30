package adapters

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/payment/stellaranchor"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
)

// The fakes embed their interfaces so only the methods these tests touch need
// implementing; anything else panics loudly rather than returning a zero value.

type fakeLoanRepo struct {
	repository.LoanRepository
	loan *models.Loan
	err  error
}

func (f *fakeLoanRepo) GetByID(_ context.Context, _ string) (*models.Loan, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.loan, nil
}

type fakeLoanSvc struct {
	loan.Service
	updates []loan.UpdateLoanRequest
}

func (f *fakeLoanSvc) Update(_ context.Context, _ string, req loan.UpdateLoanRequest) (*loan.LoanResponse, error) {
	f.updates = append(f.updates, req)
	return &loan.LoanResponse{}, nil
}

// fakeTxnSvc enforces the real service's transition table so a test cannot
// pass on a sequence the service would reject. pending -> success is invalid;
// only pending -> submitted -> success is allowed.
type fakeTxnSvc struct {
	transaction.Service
	created  []transaction.CreateTransactionRequest
	status   string
	statuses []string
}

func (f *fakeTxnSvc) Create(_ context.Context, req transaction.CreateTransactionRequest) (*transaction.TransactionResponse, error) {
	f.created = append(f.created, req)
	f.status = "pending"
	return &transaction.TransactionResponse{ID: "txn-1"}, nil
}

var allowedTransitions = map[string][]string{
	"pending":   {"submitted", "cancelled"},
	"submitted": {"success", "failed"},
}

func (f *fakeTxnSvc) Update(_ context.Context, _ string, req transaction.UpdateTransactionRequest) (*transaction.TransactionResponse, error) {
	if req.Status == nil {
		return &transaction.TransactionResponse{ID: "txn-1"}, nil
	}
	for _, ok := range allowedTransitions[f.status] {
		if ok == *req.Status {
			f.status = *req.Status
			f.statuses = append(f.statuses, *req.Status)
			return &transaction.TransactionResponse{ID: "txn-1"}, nil
		}
	}
	return nil, errors.New("invalid status transition")
}

func (f *fakeTxnSvc) offRampRows() []transaction.CreateTransactionRequest {
	var out []transaction.CreateTransactionRequest
	for _, r := range f.created {
		if r.TxType == "off_ramp" {
			out = append(out, r)
		}
	}
	return out
}

func newTestAdapter(t *testing.T, repo *fakeLoanRepo, txns *fakeTxnSvc) (*MoneyGramPollerAdapter, *fakeLoanSvc) {
	t.Helper()
	loans := &fakeLoanSvc{}
	a, err := NewMoneyGramPollerAdapter(repo, loans, txns, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	return a, loans
}

func unlockedLoan() *models.Loan {
	requestID := "MG-TX-9"
	return &models.Loan{
		UserID:          "user-1",
		AccountID:       "acct-1",
		PrincipalAmount: 500000000,
		RampRequestID:   &requestID,
	}
}

func lockedTx(status stellaranchor.Status) *stellaranchor.Transaction {
	return &stellaranchor.Transaction{
		Status:                status,
		AmountOut:             "6450.00",
		AmountOutAsset:        "iso4217:KES",
		ExternalTransactionID: "MG-REF-123",
	}
}

func TestRecordTransactionUpdate_WritesOffRampRowWhenPayoutLocks(t *testing.T) {
	repo := &fakeLoanRepo{loan: unlockedLoan()}
	txns := &fakeTxnSvc{}
	a, _ := newTestAdapter(t, repo, txns)

	require.NoError(t, a.RecordTransactionUpdate(context.Background(), "loan-1",
		lockedTx(stellaranchor.StatusPendingUserTransferComplete)))

	rows := txns.offRampRows()
	require.Len(t, rows, 1)
	assert.Equal(t, int64(645000), rows[0].Amount, "amount_out must be persisted in cents")
	assert.Equal(t, "KES", rows[0].Asset, "the iso4217: prefix must be stripped")
	require.NotNil(t, rows[0].ExternalID)
	assert.Equal(t, "MG-TX-9", *rows[0].ExternalID,
		"external_id is the provider request ID, shared by every leg")
	require.NotNil(t, rows[0].Metadata)
	assert.Contains(t, *rows[0].Metadata, "MG-REF-123",
		"the cash-pickup reference the borrower quotes must survive as metadata")
}

// The poller re-reads every loan each tick, so without a guard a locked payout
// would write a duplicate financial row every 30 seconds.
func TestRecordTransactionUpdate_DoesNotDuplicateOffRampRow(t *testing.T) {
	already := int64(645000)
	repo := &fakeLoanRepo{loan: &models.Loan{
		UserID: "user-1", AccountID: "acct-1", RampFiatAmount: &already,
	}}
	txns := &fakeTxnSvc{}
	a, _ := newTestAdapter(t, repo, txns)

	require.NoError(t, a.RecordTransactionUpdate(context.Background(), "loan-1",
		lockedTx(stellaranchor.StatusPendingUserTransferComplete)))

	assert.Empty(t, txns.offRampRows(), "ramp_fiat_amount already set; row was already written")
}

// Amounts are immutable once written, so an estimate seen before MG commits
// would be permanently wrong.
func TestRecordTransactionUpdate_NoOffRampRowBeforePayoutLocks(t *testing.T) {
	repo := &fakeLoanRepo{loan: unlockedLoan()}
	txns := &fakeTxnSvc{}
	a, loans := newTestAdapter(t, repo, txns)

	require.NoError(t, a.RecordTransactionUpdate(context.Background(), "loan-1",
		lockedTx(stellaranchor.StatusPendingUserTransferStart)))

	assert.Empty(t, txns.offRampRows(), "payout not locked yet")
	assert.Len(t, loans.updates, 1, "loan fields are still persisted")
}

// completed can be the first status observed after a restart, so it must also
// write the row rather than losing it.
func TestRecordTransactionUpdate_WritesOffRampRowOnCompleted(t *testing.T) {
	repo := &fakeLoanRepo{loan: unlockedLoan()}
	txns := &fakeTxnSvc{}
	a, _ := newTestAdapter(t, repo, txns)

	require.NoError(t, a.RecordTransactionUpdate(context.Background(), "loan-1",
		lockedTx(stellaranchor.StatusCompleted)))

	assert.Len(t, txns.offRampRows(), 1)
}

// A duplicated financial row is worse than a missing audit entry.
func TestRecordTransactionUpdate_SkipsOffRampRowWhenLoanUnreadable(t *testing.T) {
	repo := &fakeLoanRepo{err: errors.New("db down")}
	txns := &fakeTxnSvc{}
	a, _ := newTestAdapter(t, repo, txns)

	require.NoError(t, a.RecordTransactionUpdate(context.Background(), "loan-1",
		lockedTx(stellaranchor.StatusCompleted)))

	assert.Empty(t, txns.offRampRows())
}

func TestRecordSendUSDC_WritesAnchorTransferRow(t *testing.T) {
	repo := &fakeLoanRepo{loan: unlockedLoan()}
	txns := &fakeTxnSvc{}
	a, _ := newTestAdapter(t, repo, txns)

	require.NoError(t, a.RecordSendUSDC(context.Background(), "loan-1", "abc123"))

	require.Len(t, txns.created, 1)
	row := txns.created[0]
	assert.Equal(t, "anchor_transfer", row.TxType)
	assert.Equal(t, "USDC", row.Asset)
	require.NotNil(t, row.ExternalID)
	assert.Equal(t, "MG-TX-9", *row.ExternalID)
	assert.Equal(t, int64(500000000), row.Amount, "the on-chain leg is the USDC principal")
	require.NotNil(t, row.StellarTxHash)
	assert.Equal(t, "abc123", *row.StellarTxHash)

	// The payment is confirmed on-ledger before RecordSendUSDC is reached, so
	// the row must not be left pending. pending -> success is rejected by the
	// service, so this only holds if the submitted step is walked first.
	assert.Equal(t, []string{"submitted", "success"}, txns.statuses)
	assert.Equal(t, "success", txns.status)
}

// A locked payout is committed but not yet collected.
func TestRecordTransactionUpdate_OffRampRowIsSubmittedUntilCollected(t *testing.T) {
	repo := &fakeLoanRepo{loan: unlockedLoan()}
	txns := &fakeTxnSvc{}
	a, _ := newTestAdapter(t, repo, txns)

	require.NoError(t, a.RecordTransactionUpdate(context.Background(), "loan-1",
		lockedTx(stellaranchor.StatusPendingUserTransferComplete)))

	assert.Equal(t, "submitted", txns.status)
}

func TestRecordTransactionUpdate_OffRampRowSucceedsOnCompleted(t *testing.T) {
	repo := &fakeLoanRepo{loan: unlockedLoan()}
	txns := &fakeTxnSvc{}
	a, _ := newTestAdapter(t, repo, txns)

	require.NoError(t, a.RecordTransactionUpdate(context.Background(), "loan-1",
		lockedTx(stellaranchor.StatusCompleted)))

	assert.Equal(t, []string{"submitted", "success"}, txns.statuses)
}

// The USDC has already left the treasury, so bookkeeping must not fail the
// send — a returned error would leave the claim in a state that invites a
// re-send.
func TestRecordSendUSDC_BookkeepingFailureDoesNotFailSend(t *testing.T) {
	repo := &fakeLoanRepo{err: errors.New("db down")}
	txns := &fakeTxnSvc{}
	a, _ := newTestAdapter(t, repo, txns)

	assert.NoError(t, a.RecordSendUSDC(context.Background(), "loan-1", "abc123"))
	assert.Empty(t, txns.created)
}

// One MoneyGram withdrawal settles as several legs, all carrying the provider's
// request ID. The old uniqueness rule on external_id made recording the full
// lifecycle impossible; nothing may reintroduce it.
func TestLegsOfOneWithdrawalShareExternalID(t *testing.T) {
	repo := &fakeLoanRepo{loan: unlockedLoan()}
	txns := &fakeTxnSvc{}
	a, _ := newTestAdapter(t, repo, txns)
	ctx := context.Background()

	require.NoError(t, a.RecordSendUSDC(ctx, "loan-1", "abc123"))
	require.NoError(t, a.RecordTransactionUpdate(ctx, "loan-1",
		lockedTx(stellaranchor.StatusPendingUserTransferComplete)))

	byType := map[string]string{}
	for _, r := range txns.created {
		require.NotNil(t, r.ExternalID, "every provider leg must carry a reference: %s", r.TxType)
		byType[r.TxType] = *r.ExternalID
	}

	require.Contains(t, byType, "anchor_transfer")
	require.Contains(t, byType, "off_ramp")
	assert.Equal(t, byType["anchor_transfer"], byType["off_ramp"],
		"both legs belong to the same anchor transaction")
}

func TestRecordTransactionUpdate_RecordsMoneyGramFeeAsPartnerFeeUSD(t *testing.T) {
	cases := []struct {
		name string
		fee  *stellaranchor.FeeDetails
		want *int64
	}{
		{"withdrawal iso4217:USDC", &stellaranchor.FeeDetails{Total: "3.00", Asset: "iso4217:USDC"}, ptrInt64(300)},
		{"deposit iso4217:USD", &stellaranchor.FeeDetails{Total: "3.00", Asset: "iso4217:USD"}, ptrInt64(300)},
		{"sep-38 stellar asset", &stellaranchor.FeeDetails{Total: "0.00", Asset: "stellar:USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"}, ptrInt64(0)},
		{"local-currency fee is not a USD amount", &stellaranchor.FeeDetails{Total: "380", Asset: "iso4217:KES"}, nil},
		{"unparseable total", &stellaranchor.FeeDetails{Total: "", Asset: "iso4217:USDC"}, nil},
		{"no fee_details", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, loans := newTestAdapter(t, &fakeLoanRepo{loan: unlockedLoan()}, &fakeTxnSvc{})
			tx := lockedTx(stellaranchor.StatusPendingUserTransferStart)
			tx.FeeDetails = tc.fee

			require.NoError(t, a.RecordTransactionUpdate(context.Background(), "loan-1", tx))

			require.Len(t, loans.updates, 1)
			got := loans.updates[0]
			assert.Equal(t, tc.want, got.PartnerFeeUSD)
			assert.Nil(t, got.ServiceFeeUSD, "service_fee_usd is charged to the borrower; MG's fee payer is unconfirmed")
			assert.Nil(t, got.ServiceFeeLocal)
		})
	}
}

func TestRecordTransactionUpdate_DeliveredAmountIsGrossAmountOut(t *testing.T) {
	a, loans := newTestAdapter(t, &fakeLoanRepo{loan: unlockedLoan()}, &fakeTxnSvc{})
	tx := lockedTx(stellaranchor.StatusPendingUserTransferStart)
	tx.FeeDetails = &stellaranchor.FeeDetails{Total: "3.00", Asset: "iso4217:USDC"}

	require.NoError(t, a.RecordTransactionUpdate(context.Background(), "loan-1", tx))

	require.Len(t, loans.updates, 1)
	require.NotNil(t, loans.updates[0].DeliveredAmountLocal)
	assert.Equal(t, int64(645000), *loans.updates[0].DeliveredAmountLocal)
}

func ptrInt64(v int64) *int64 { return &v }

func TestDecimalToCents(t *testing.T) {
	cases := []struct {
		in     string
		want   int64
		wantOK bool
	}{
		{"19.49", 1949, true},
		{"0.29", 29, true},
		{"3.00", 300, true},
		{"2508", 250800, true},
		{"6450.5", 645050, true},
		{"23.4300000", 2343, true},
		{"1.", 100, true},
		{"23.4301780", 0, false},
		{"1.005", 0, false},
		{"-3.00", 0, false},
		{"3.00xyz", 0, false},
		{"1e3", 0, false},
		{".50", 0, false},
		{"", 0, false},
		{" 3.00", 0, false},
		{"92233720368547758.07", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := decimalToCents(tc.in)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}
