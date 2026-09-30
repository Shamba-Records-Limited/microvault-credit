package adapters

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	users "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	"github.com/Shamba-Records-Limited/microvault/pkg/urlshortener"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
)

type recordingLoanNotifier struct {
	contracts.LoanNotifier
	moreInfo          []contracts.LoanNotification
	windowExpiring    []contracts.LoanNotification
	repaymentReceived []contracts.LoanNotification
	repaid            []contracts.LoanNotification
}

func (r *recordingLoanNotifier) NotifyRepaymentMoreInfo(_ context.Context, n contracts.LoanNotification) error {
	r.moreInfo = append(r.moreInfo, n)
	return nil
}

func (r *recordingLoanNotifier) NotifyRepaymentWindowExpiring(_ context.Context, n contracts.LoanNotification) error {
	r.windowExpiring = append(r.windowExpiring, n)
	return nil
}

func (r *recordingLoanNotifier) NotifyRepaymentReceived(_ context.Context, n contracts.LoanNotification) error {
	r.repaymentReceived = append(r.repaymentReceived, n)
	return nil
}

func (r *recordingLoanNotifier) NotifyLoanRepaid(_ context.Context, n contracts.LoanNotification) error {
	r.repaid = append(r.repaid, n)
	return nil
}

const moneyGramInfoURL = "https://extramps.moneygram.com/transaction-status?transaction_id=4a93bfcf&token=eyJhbGciOiJIUzI1NiJ9"

func loanWithMoreInfo(shortCode string) *models.Loan {
	rawURL := moneyGramInfoURL
	return &models.Loan{
		UserID:                "user-1",
		RampMoreInfoURL:       &rawURL,
		RampMoreInfoShortCode: &shortCode,
		User:                  &users.User{MobileNumber: "+254711222111"},
	}
}

func newTestRepaymentNotifier(t *testing.T, loanRow *models.Loan, baseURL string) (*RepaymentNotifierAdapter, *recordingLoanNotifier) {
	t.Helper()
	return newTestRepaymentNotifierWith(t, loanRow, baseURL, nil)
}

func newTestRepaymentNotifierWith(
	t *testing.T,
	loanRow *models.Loan,
	baseURL string,
	shortener urlshortener.Shortener,
) (*RepaymentNotifierAdapter, *recordingLoanNotifier) {
	t.Helper()
	notifier := &recordingLoanNotifier{}
	a, err := NewRepaymentNotifierAdapter(
		&fakeLoanRepo{loan: loanRow},
		notifier,
		baseURL,
		shortener,
		nil,
		slog.New(slog.DiscardHandler),
	)
	require.NoError(t, err)
	return a, notifier
}

// With no shortener configured the SMS carries the /r/{code} redirect.
// MoneyGram's own URL carries a JWT in its query string and would split a
// one-segment SMS into four, so the raw URL is never sent as-is.
func TestNotifyRepaymentMoreInfo_RedirectWhenNoShortener(t *testing.T) {
	a, notifier := newTestRepaymentNotifier(t, loanWithMoreInfo("Xk9f2aQ7ab"), "https://microvault.outray.app")

	require.NoError(t, a.NotifyRepaymentMoreInfo(context.Background(), "loan-1"))

	require.Len(t, notifier.moreInfo, 1)
	assert.Equal(t, "https://microvault.outray.app/r/Xk9f2aQ7ab", notifier.moreInfo[0].InteractiveURL)
}

// The whole point of this message is the link. Copy that trails off after
// "open this for your MoneyGram payment details:" tells the borrower nothing
// while consuming the one-shot send marker.
func TestNotifyRepaymentMoreInfo_RefusesToSendWithoutALink(t *testing.T) {
	a, notifier := newTestRepaymentNotifier(t, loanWithMoreInfo(""), "https://microvault.outray.app")

	err := a.NotifyRepaymentMoreInfo(context.Background(), "loan-1")

	require.Error(t, err)
	assert.Empty(t, notifier.moreInfo)
}

// RepaymentWindowExpiring renders InteractiveURL too, and the adapter never
// populated it — every reminder went out with an empty line where the link
// belongs.
func TestNotifyRepaymentReminder_CarriesTheLink(t *testing.T) {
	a, notifier := newTestRepaymentNotifier(t, loanWithMoreInfo("Xk9f2aQ7ab"), "https://microvault.outray.app")

	require.NoError(t, a.NotifyRepaymentReminder(context.Background(), "loan-1"))

	require.Len(t, notifier.windowExpiring, 1)
	assert.Equal(t, "https://microvault.outray.app/r/Xk9f2aQ7ab", notifier.windowExpiring[0].InteractiveURL)
}

// The adapter reads the loan back through GetByID, which was the one Get*
// method in the repository not preloading User. Every notification on this
// rail failed with "loan has no phone number to notify" — invisibly, because
// the reference SMS never fired until the more-info fallback landed.
func TestNotifyRepayment_RequiresThePreloadedUser(t *testing.T) {
	loanRow := loanWithMoreInfo("Xk9f2aQ7ab")
	loanRow.User = nil // what an un-preloaded row looks like
	a, notifier := newTestRepaymentNotifier(t, loanRow, "https://microvault.outray.app")

	err := a.NotifyRepaymentMoreInfo(context.Background(), "loan-1")

	require.Error(t, err)
	assert.Equal(t, pkgErrors.CodeMissingPhoneNumber, corridorCode(t, err))
	assert.Empty(t, notifier.moreInfo)
}

func TestNotifyRepaymentMoreInfo_NoBaseURLConfigured(t *testing.T) {
	a, _ := newTestRepaymentNotifier(t, loanWithMoreInfo("Xk9f2aQ7ab"), "")

	assert.Error(t, a.NotifyRepaymentMoreInfo(context.Background(), "loan-1"))
}

// dub is pointed at MoneyGram's own URL, not at our redirect, so the link
// preview in the borrower's inbox names MoneyGram. The /r/{code} hop adds
// nothing here: MoneyGram expires the session itself.
func TestNotifyRepaymentMoreInfo_ShortensTheMoneyGramURL(t *testing.T) {
	sh := &fakeShortener{short: "https://dub.sh/mg7x2"}
	a, notifier := newTestRepaymentNotifierWith(t, loanWithMoreInfo("Xk9f2aQ7ab"), "https://microvault.outray.app", sh)

	require.NoError(t, a.NotifyRepaymentMoreInfo(context.Background(), "loan-1"))

	assert.Equal(t, moneyGramInfoURL, sh.gotURL, "dub shortens the destination, not our redirect")
	require.Len(t, notifier.moreInfo, 1)
	assert.Equal(t, "https://dub.sh/mg7x2", notifier.moreInfo[0].InteractiveURL)
}

// A shortener outage must not cost the borrower their instructions — the
// redirect resolves to the same place.
func TestNotifyRepaymentMoreInfo_FallsBackToTheRedirect(t *testing.T) {
	sh := &fakeShortener{err: errors.New("dub is down")}
	a, notifier := newTestRepaymentNotifierWith(t, loanWithMoreInfo("Xk9f2aQ7ab"), "https://microvault.outray.app", sh)

	require.NoError(t, a.NotifyRepaymentMoreInfo(context.Background(), "loan-1"))

	require.Len(t, notifier.moreInfo, 1)
	assert.Equal(t, "https://microvault.outray.app/r/Xk9f2aQ7ab", notifier.moreInfo[0].InteractiveURL)
}

// loanWithPayoff builds a fixture with a locked payoff, for the amount/
// currency tests below. rampProvider drives which offramp the FX resolution
// tries first.
func loanWithPayoff(payoffStroops, receivedStroops int64, rampProvider string) *models.Loan {
	payoff := payoffStroops
	ref := "SHHS5W6PH"
	loan := &models.Loan{
		ID:                     "loan-1",
		LoanReference:          &ref,
		RepaymentPayoffStroops: &payoff,
		User:                   &users.User{MobileNumber: "+254711222111"},
	}
	if receivedStroops > 0 {
		received := receivedStroops
		loan.RepaymentReceivedStroops = &received
	}
	if rampProvider != "" {
		loan.RampProvider = &rampProvider
	}
	return loan
}

func newTestRepaymentNotifierWithOffRamps(t *testing.T, loanRow *models.Loan, offRamps *offramp.Registry) (*RepaymentNotifierAdapter, *recordingLoanNotifier) {
	t.Helper()
	notifier := &recordingLoanNotifier{}
	a, err := NewRepaymentNotifierAdapter(
		&fakeLoanRepo{loan: loanRow},
		notifier,
		"",
		nil,
		offRamps,
		slog.New(slog.DiscardHandler),
	)
	require.NoError(t, err)
	return a, notifier
}

// The bug this locks in: an SMS reading "Payment of USDC 8.35 received" is
// meaningless to a borrower who has never heard of USDC. It must render in
// the loan's local currency whenever a rate is reachable.
func TestNotifyRepaymentReceived_RendersInLocalCurrency(t *testing.T) {
	loanRow := loanWithPayoff(10_000_000, 0, "yellowcard") // 1.0 USDC payoff, fully received
	offRamps := offramp.NewRegistry()
	require.NoError(t, offRamps.Register(&fakeQuoteProvider{id: offramp.ProviderYellowCard, rate: 129.0}))
	a, notifier := newTestRepaymentNotifierWithOffRamps(t, loanRow, offRamps)

	require.NoError(t, a.NotifyRepaymentReceived(context.Background(), "loan-1"))

	require.Len(t, notifier.repaymentReceived, 1)
	n := notifier.repaymentReceived[0]
	assert.Equal(t, "KES", n.DisplayCurrency)
	assert.InDelta(t, 129.0, n.DisplayAmount, 0.01)
}

func TestNotifyRepaymentReceivedAmount_UsesTheObservedAmountOverThePayoff(t *testing.T) {
	loanRow := loanWithPayoff(10_000_000, 0, "yellowcard")
	offRamps := offramp.NewRegistry()
	require.NoError(t, offRamps.Register(&fakeQuoteProvider{id: offramp.ProviderYellowCard, rate: 129.0}))
	a, notifier := newTestRepaymentNotifierWithOffRamps(t, loanRow, offRamps)

	require.NoError(t, a.NotifyRepaymentReceivedAmount(context.Background(), "loan-1", 5))

	require.Len(t, notifier.repaymentReceived, 1)
	n := notifier.repaymentReceived[0]
	assert.Equal(t, "KES", n.DisplayCurrency)
	assert.Equal(t, float64(5), n.DisplayAmount)
}

func TestNotifyRepaymentReceivedAmount_ZeroFallsBackToThePayoff(t *testing.T) {
	loanRow := loanWithPayoff(10_000_000, 0, "yellowcard")
	offRamps := offramp.NewRegistry()
	require.NoError(t, offRamps.Register(&fakeQuoteProvider{id: offramp.ProviderYellowCard, rate: 129.0}))
	a, notifier := newTestRepaymentNotifierWithOffRamps(t, loanRow, offRamps)

	require.NoError(t, a.NotifyRepaymentReceivedAmount(context.Background(), "loan-1", 0))

	require.Len(t, notifier.repaymentReceived, 1)
	assert.InDelta(t, 129.0, notifier.repaymentReceived[0].DisplayAmount, 0.01)
}

// A loan disbursed in a non-default currency renders in that currency, not
// the KES default.
func TestNotifyRepaymentReceived_UsesTheLoansOwnCurrency(t *testing.T) {
	loanRow := loanWithPayoff(10_000_000, 0, "yellowcard")
	fiatCurr := "UGX"
	loanRow.RampFiatCurr = &fiatCurr
	offRamps := offramp.NewRegistry()
	require.NoError(t, offRamps.Register(&fakeQuoteProvider{id: offramp.ProviderYellowCard, rate: 3700.0}))
	a, notifier := newTestRepaymentNotifierWithOffRamps(t, loanRow, offRamps)

	require.NoError(t, a.NotifyRepaymentReceived(context.Background(), "loan-1"))

	require.Len(t, notifier.repaymentReceived, 1)
	assert.Equal(t, "UGX", notifier.repaymentReceived[0].DisplayCurrency)
}

// No FX rate anywhere (no registry, or no provider registered) must not
// fail the send — it falls back to USDC, matching the previous behaviour,
// rather than leaving the borrower with no notification at all.
func TestNotifyRepaymentReceived_FallsBackToUSDCWhenNoFXAvailable(t *testing.T) {
	loanRow := loanWithPayoff(10_000_000, 0, "")
	a, notifier := newTestRepaymentNotifierWithOffRamps(t, loanRow, nil)

	require.NoError(t, a.NotifyRepaymentReceived(context.Background(), "loan-1"))

	require.Len(t, notifier.repaymentReceived, 1)
	assert.Equal(t, "USDC", notifier.repaymentReceived[0].DisplayCurrency)
}

// RemainingBalance must reflect what is actually still owed — see the "USDC
// 0.00" bug report this fixed: the field was never populated at all before,
// so it always read zero regardless of the loan's real state.
func TestNotifyRepaymentReceived_RemainingBalanceReflectsPartialPayment(t *testing.T) {
	loanRow := loanWithPayoff(10_000_000, 4_000_000, "yellowcard") // paid 0.4 of 1.0 USDC
	offRamps := offramp.NewRegistry()
	require.NoError(t, offRamps.Register(&fakeQuoteProvider{id: offramp.ProviderYellowCard, rate: 129.0}))
	a, notifier := newTestRepaymentNotifierWithOffRamps(t, loanRow, offRamps)

	require.NoError(t, a.NotifyRepaymentReceived(context.Background(), "loan-1"))

	require.Len(t, notifier.repaymentReceived, 1)
	// Remaining: 0.6 USDC * 129 KES/USDC = 77.4 KES.
	assert.InDelta(t, 77.4, notifier.repaymentReceived[0].RemainingBalance, 0.01)
}

func TestNotifyLoanRepaid_RendersInLocalCurrency(t *testing.T) {
	loanRow := loanWithPayoff(10_000_000, 10_000_000, "yellowcard")
	offRamps := offramp.NewRegistry()
	require.NoError(t, offRamps.Register(&fakeQuoteProvider{id: offramp.ProviderYellowCard, rate: 129.0}))
	a, notifier := newTestRepaymentNotifierWithOffRamps(t, loanRow, offRamps)

	require.NoError(t, a.NotifyLoanRepaid(context.Background(), "loan-1"))

	require.Len(t, notifier.repaid, 1)
	assert.Equal(t, "KES", notifier.repaid[0].DisplayCurrency)
	assert.InDelta(t, 0, notifier.repaid[0].RemainingBalance, 0.01)
}

func TestNotifyLoanRepaidAmount_UsesTheObservedAmountOverThePayoff(t *testing.T) {
	loanRow := loanWithPayoff(10_000_000, 10_000_000, "yellowcard")
	offRamps := offramp.NewRegistry()
	require.NoError(t, offRamps.Register(&fakeQuoteProvider{id: offramp.ProviderYellowCard, rate: 129.0}))
	a, notifier := newTestRepaymentNotifierWithOffRamps(t, loanRow, offRamps)

	require.NoError(t, a.NotifyLoanRepaidAmount(context.Background(), "loan-1", 5))

	require.Len(t, notifier.repaid, 1)
	assert.Equal(t, "KES", notifier.repaid[0].DisplayCurrency)
	assert.Equal(t, float64(5), notifier.repaid[0].DisplayAmount)
}
