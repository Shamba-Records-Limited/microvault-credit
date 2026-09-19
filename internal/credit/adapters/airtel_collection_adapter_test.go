package adapters

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/airtel"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/airtel/airtelstub"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/cashin"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
)

func newTestAirtelAdapter(t *testing.T, l *models.Loan) (*AirtelCollectionAdapter, *airtelstub.Stub, *fakeLoanSvc) {
	t.Helper()

	stub := airtelstub.New(t, airtelstub.WithCredentials("id", "secret"))
	client, err := airtel.New(airtel.Config{
		Environment:  airtel.EnvironmentStaging,
		ClientID:     "id",
		ClientSecret: "secret",
		BaseURL:      stub.URL(),
	})
	require.NoError(t, err)

	loans := &fakeLoanSvc{}
	a, err := NewAirtelCollectionAdapter(AirtelCollectionAdapterDeps{
		Client:  client,
		Repo:    &fakeSTKLoanRepo{loan: l},
		LoanSvc: loans,
		Config: config.AirtelConfig{
			EnquiryDelay: config.EnquiryDelayFloor,
			PollInterval: time.Minute,
		},
	})
	require.NoError(t, err)
	a.now = func() time.Time { return time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC) }
	return a, stub, loans
}

func airtelLoan() *models.Loan {
	return &models.Loan{ID: "loan-1", LoanReference: lo.ToPtr("MV12345678")}
}

// Airtel Collection is push-only. Collector is the registry's mandatory
// capability, so the method exists — and refuses.
func TestAirtelCollect_Refuses(t *testing.T) {
	a, _, _ := newTestAirtelAdapter(t, airtelLoan())

	_, err := a.Collect(context.Background(), cashin.Request{LoanID: "loan-1", AmountMinor: 15_000})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no passive collection")
}

func TestAirtelPrompt_PushesAndMarksInitiated(t *testing.T) {
	a, stub, loans := newTestAirtelAdapter(t, airtelLoan())

	res, err := a.Prompt(context.Background(), cashin.PromptRequest{
		LoanID:            "loan-1",
		Payer:             "0733123456",
		AmountKES:         500,
		AmountUSDCStroops: 38_000_000,
	})
	require.NoError(t, err)

	payload, ok := res.Provider.(airtel.PromptPayload)
	require.True(t, ok)
	assert.Equal(t, int64(500), payload.PromptedKES)

	// The id we minted is what reached Airtel, and it is unique per attempt.
	txns := stub.Transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, payload.TransactionID, txns[0].ID)
	assert.True(t, strings.HasPrefix(txns[0].ID, "MV12345678-"),
		"the transaction id should carry the loan reference: %q", txns[0].ID)
	assert.Equal(t, "733123456", txns[0].MSISDN, "the country code must not be sent")

	require.Len(t, loans.updates, 1)
	update := loans.updates[0]
	require.NotNil(t, update.RepaymentProvider)
	assert.Equal(t, models.LoanRepaymentProviderAirtel, *update.RepaymentProvider)
	require.NotNil(t, update.RepaymentStatus)
	assert.Equal(t, models.LoanRepaymentStatusInitiated, *update.RepaymentStatus)
	require.NotNil(t, update.RepaymentAirtelTxnID)
	assert.Equal(t, payload.TransactionID, *update.RepaymentAirtelTxnID)

	// The payoff lock is the USDC figure, never derived from the KES pushed.
	require.NotNil(t, update.RepaymentPayoffStroops)
	assert.Equal(t, int64(38_000_000), *update.RepaymentPayoffStroops)
}

// The first enquiry is scheduled at Airtel's documented floor, not at the
// shorter poll interval: asking sooner returns nothing and spends a call.
func TestAirtelPrompt_SchedulesTheFirstEnquiryAtTheFloor(t *testing.T) {
	a, _, loans := newTestAirtelAdapter(t, airtelLoan())

	_, err := a.Prompt(context.Background(), cashin.PromptRequest{
		LoanID: "loan-1", Payer: "0733123456", AmountKES: 500,
	})
	require.NoError(t, err)

	require.Len(t, loans.updates, 1)
	require.NotNil(t, loans.updates[0].RepaymentNextPollAt)
	assert.Equal(t, a.now().Add(config.EnquiryDelayFloor), *loans.updates[0].RepaymentNextPollAt)
}

func TestAirtelPrompt_RefusesASecondPrompt(t *testing.T) {
	l := airtelLoan()
	l.RepaymentStatus = models.LoanRepaymentStatusInitiated
	a, _, _ := newTestAirtelAdapter(t, l)

	_, err := a.Prompt(context.Background(), cashin.PromptRequest{
		LoanID: "loan-1", Payer: "0733123456", AmountKES: 500,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already in flight")
}

func TestAirtelPrompt_RequiresAReference(t *testing.T) {
	a, _, _ := newTestAirtelAdapter(t, &models.Loan{ID: "loan-1"})

	_, err := a.Prompt(context.Background(), cashin.PromptRequest{
		LoanID: "loan-1", Payer: "0733123456", AmountKES: 500,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "short reference")
}

// The staging override replaces the charged amount but must never touch the
// payoff lock, which is what the vault leg later reads.
func TestAirtelPrompt_StagingOverrideDoesNotMoveThePayoffLock(t *testing.T) {
	a, stub, loans := newTestAirtelAdapter(t, airtelLoan())
	a.cfg.PromptAmountKES = 1

	_, err := a.Prompt(context.Background(), cashin.PromptRequest{
		LoanID: "loan-1", Payer: "0733123456", AmountKES: 500, AmountUSDCStroops: 38_000_000,
	})
	require.NoError(t, err)

	txns := stub.Transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, int64(1), txns[0].AmountKES, "the handset should be charged the override")

	require.NotNil(t, loans.updates[0].RepaymentPayoffStroops)
	assert.Equal(t, int64(38_000_000), *loans.updates[0].RepaymentPayoffStroops)
}

func TestAirtelStatus_ReportsTheEnquiry(t *testing.T) {
	a, stub, _ := newTestAirtelAdapter(t, airtelLoan())

	_, err := a.Prompt(context.Background(), cashin.PromptRequest{
		LoanID: "loan-1", Payer: "0733123456", AmountKES: 500,
	})
	require.NoError(t, err)
	txnID := stub.Transactions()[0].ID

	status, err := a.Status(context.Background(), cashin.ProviderRef{ID: txnID})
	require.NoError(t, err)
	assert.True(t, status.Succeeded)

	payload, ok := status.Provider.(airtel.CollectionPayload)
	require.True(t, ok)
	assert.NotEmpty(t, payload.AirtelMoneyID, "a settled enquiry discloses the receipt")
}

// A refund is keyed by Airtel's receipt, not the id we minted, so it is
// impossible until an enquiry or callback has disclosed one.
func TestAirtelReverse_RequiresTheReceipt(t *testing.T) {
	a, _, _ := newTestAirtelAdapter(t, airtelLoan())

	_, err := a.Reverse(context.Background(), cashin.ProviderRef{}, 0, "duplicate")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no airtel money id")
}

func TestAirtelReverse_RefundsByReceipt(t *testing.T) {
	a, stub, _ := newTestAirtelAdapter(t, airtelLoan())

	_, err := a.Prompt(context.Background(), cashin.PromptRequest{
		LoanID: "loan-1", Payer: "0733123456", AmountKES: 500,
	})
	require.NoError(t, err)
	receipt := stub.Transactions()[0].AirtelMoneyID
	require.NotEmpty(t, receipt)

	res, err := a.Reverse(context.Background(), cashin.ProviderRef{ID: receipt}, 0, "duplicate")
	require.NoError(t, err)
	assert.Equal(t, receipt, res.Reference)

	payload, ok := res.Provider.(airtel.RefundPayload)
	require.True(t, ok)
	assert.Equal(t, receipt, payload.AirtelMoneyID)
}

func TestAirtelAdapter_ID(t *testing.T) {
	a, _, _ := newTestAirtelAdapter(t, airtelLoan())
	assert.Equal(t, cashin.ProviderAirtel, a.ID())
}
