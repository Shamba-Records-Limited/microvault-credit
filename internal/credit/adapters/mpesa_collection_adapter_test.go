package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/cashin"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/mpesa"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/mpesa/darajastub"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
)

func newTestCollectionAdapter(t *testing.T, loan *models.Loan) (*MpesaCollectionAdapter, *darajastub.Stub, *fakeLoanSvc) {
	t.Helper()
	stub := darajastub.New(t, darajastub.WithConsumerCredentials("k", "s"))
	client, err := mpesa.New(mpesa.Config{
		Environment:         mpesa.EnvironmentSandbox,
		ConsumerKey:         "k",
		ConsumerSecret:      "s",
		CollectionShortcode: 174379,
		Passkey:             "stub-passkey",
		BaseURL:             stub.URL(),
		Certificate:         stub.Certificate(),
	})
	require.NoError(t, err)
	loans := &fakeLoanSvc{}
	a, err := NewMpesaCollectionAdapter(MpesaCollectionAdapterDeps{
		Client:  client,
		Repo:    &fakeSTKLoanRepo{loan: loan},
		LoanSvc: loans,
		Config: config.MpesaConfig{
			CollectionShortcode: 174379,
			STKPollInterval:     5 * time.Second,
			CallbackBaseURL:     "https://x.test",
			CallbackSlug:        "slug",
		},
	})
	require.NoError(t, err)
	a.now = func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) }
	return a, stub, loans
}

func referencedLoan() *models.Loan {
	return &models.Loan{ID: "loan-1", LoanReference: lo.ToPtr("MV12345678")}
}

func TestCollect_BuildsThePayBillInstruction(t *testing.T) {
	a, _, _ := newTestCollectionAdapter(t, referencedLoan())

	res, err := a.Collect(context.Background(), cashin.Request{
		LoanID:      "loan-1",
		AmountMinor: 15_000,
	})

	require.NoError(t, err)
	assert.Equal(t, "loan-1", res.LoanID)
	assert.Equal(t, "MV12345678", res.Reference)
	assert.Equal(t, int64(15_000), res.Amount)
	payload, ok := res.Provider.(mpesa.PayBillPayload)
	require.True(t, ok)
	assert.Equal(t, uint(174379), payload.Shortcode)
	assert.Equal(t, "MV12345678", payload.AccountReference)
}

func TestCollect_RejectsThePromptMethod(t *testing.T) {
	a, _, _ := newTestCollectionAdapter(t, referencedLoan())

	_, err := a.Collect(context.Background(), cashin.Request{
		LoanID:           "loan-1",
		AmountMinor:      15_000,
		CollectionMethod: cashin.CollectionMethodPrompt,
	})

	require.Error(t, err)
}

func TestCollect_RequiresALoanReference(t *testing.T) {
	a, _, _ := newTestCollectionAdapter(t, &models.Loan{ID: "loan-1"})

	_, err := a.Collect(context.Background(), cashin.Request{LoanID: "loan-1", AmountMinor: 15_000})

	require.Error(t, err)
}

func TestPrompt_RecordsTheCheckoutOnTheLoan(t *testing.T) {
	a, _, loans := newTestCollectionAdapter(t, referencedLoan())

	res, err := a.Prompt(context.Background(), cashin.PromptRequest{
		LoanID:    "loan-1",
		Payer:     "254712345678",
		AmountKES: 150,
	})

	require.NoError(t, err)
	assert.Equal(t, "loan-1", res.LoanID)
	payload, ok := res.Provider.(mpesa.ExpressPayload)
	require.True(t, ok)
	assert.NotEmpty(t, payload.CheckoutRequestID)
	assert.NotEmpty(t, payload.CustomerMessage)
	assert.Equal(t, int64(150), payload.PromptedAmountKES)

	require.Len(t, loans.updates, 1)
	upd := loans.updates[0]
	require.NotNil(t, upd.RepaymentProvider)
	assert.Equal(t, models.LoanRepaymentProviderMpesa, *upd.RepaymentProvider)
	require.NotNil(t, upd.RepaymentStatus)
	assert.Equal(t, models.LoanRepaymentStatusInitiated, *upd.RepaymentStatus)
	require.NotNil(t, upd.RepaymentMpesaCheckoutID)
	assert.Equal(t, payload.CheckoutRequestID, *upd.RepaymentMpesaCheckoutID)
	require.NotNil(t, upd.RepaymentNextPollAt)
	assert.Equal(t, time.Date(2026, 9, 7, 12, 0, 5, 0, time.UTC), *upd.RepaymentNextPollAt)
}

func TestPrompt_DeclinedBeforeAnyStateIsWritten(t *testing.T) {
	a, _, loans := newTestCollectionAdapter(t, referencedLoan())

	_, err := a.Prompt(context.Background(), cashin.PromptRequest{
		LoanID:    "loan-1",
		Payer:     "123",
		AmountKES: 150,
	})

	require.Error(t, err)
	assert.Empty(t, loans.updates, "a rejected push must not mark the repayment initiated")
}

func TestPromptThenStatus_ResolvedByQuery(t *testing.T) {
	a, stub, _ := newTestCollectionAdapter(t, referencedLoan())
	res, err := a.Prompt(context.Background(), cashin.PromptRequest{
		LoanID: "loan-1", Payer: "254712345678", AmountKES: 150,
	})
	require.NoError(t, err)
	checkout := res.Provider.(mpesa.ExpressPayload).CheckoutRequestID

	_, err = a.Status(context.Background(), cashin.ProviderRef{ID: checkout})
	require.Error(t, err, "an unresolved checkout is still in flight")

	stub.CompleteSTK(checkout)
	status, err := a.Status(context.Background(), cashin.ProviderRef{ID: checkout})
	require.NoError(t, err)
	assert.True(t, status.Succeeded)
	assert.Equal(t, checkout, status.Reference)
}
