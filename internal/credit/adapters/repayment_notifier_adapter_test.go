package adapters

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Shamba-Records-Limited/microvault/pkg/contracts"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
	users "github.com/Shamba-Records-Limited/microvault/pkg/models"
	"github.com/Shamba-Records-Limited/microvault/pkg/urlshortener"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
)

type recordingLoanNotifier struct {
	contracts.LoanNotifier
	moreInfo       []contracts.LoanNotification
	windowExpiring []contracts.LoanNotification
}

func (r *recordingLoanNotifier) NotifyRepaymentMoreInfo(_ context.Context, n contracts.LoanNotification) error {
	r.moreInfo = append(r.moreInfo, n)
	return nil
}

func (r *recordingLoanNotifier) NotifyRepaymentWindowExpiring(_ context.Context, n contracts.LoanNotification) error {
	r.windowExpiring = append(r.windowExpiring, n)
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
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	require.NoError(t, err)
	return a, notifier
}

// With no shortener configured the SMS carries the /r/{code} redirect.
// MoneyGram's own URL carries a JWT in its query string and would split a
// one-segment SMS into four, so the raw URL is never sent as-is.
func TestNotifyRepaymentMoreInfo_RedirectWhenNoShortener(t *testing.T) {
	a, notifier := newTestRepaymentNotifier(t, loanWithMoreInfo("Xk9f2aQ7ab"), "https://microvault.outray.app")

	require.NoError(t, a.NotifyRepaymentMoreInfo("loan-1"))

	require.Len(t, notifier.moreInfo, 1)
	assert.Equal(t, "https://microvault.outray.app/r/Xk9f2aQ7ab", notifier.moreInfo[0].InteractiveURL)
}

// The whole point of this message is the link. Copy that trails off after
// "open this for your MoneyGram payment details:" tells the borrower nothing
// while consuming the one-shot send marker.
func TestNotifyRepaymentMoreInfo_RefusesToSendWithoutALink(t *testing.T) {
	a, notifier := newTestRepaymentNotifier(t, loanWithMoreInfo(""), "https://microvault.outray.app")

	err := a.NotifyRepaymentMoreInfo("loan-1")

	require.Error(t, err)
	assert.Empty(t, notifier.moreInfo)
}

// RepaymentWindowExpiring renders InteractiveURL too, and the adapter never
// populated it — every reminder went out with an empty line where the link
// belongs.
func TestNotifyRepaymentReminder_CarriesTheLink(t *testing.T) {
	a, notifier := newTestRepaymentNotifier(t, loanWithMoreInfo("Xk9f2aQ7ab"), "https://microvault.outray.app")

	require.NoError(t, a.NotifyRepaymentReminder("loan-1"))

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

	err := a.NotifyRepaymentMoreInfo("loan-1")

	require.Error(t, err)
	assert.Equal(t, pkgErrors.CodeMissingPhoneNumber, corridorCode(t, err))
	assert.Empty(t, notifier.moreInfo)
}

func TestNotifyRepaymentMoreInfo_NoBaseURLConfigured(t *testing.T) {
	a, _ := newTestRepaymentNotifier(t, loanWithMoreInfo("Xk9f2aQ7ab"), "")

	assert.Error(t, a.NotifyRepaymentMoreInfo("loan-1"))
}

// dub is pointed at MoneyGram's own URL, not at our redirect, so the link
// preview in the borrower's inbox names MoneyGram. The /r/{code} hop adds
// nothing here: MoneyGram expires the session itself.
func TestNotifyRepaymentMoreInfo_ShortensTheMoneyGramURL(t *testing.T) {
	sh := &fakeShortener{short: "https://dub.sh/mg7x2"}
	a, notifier := newTestRepaymentNotifierWith(t, loanWithMoreInfo("Xk9f2aQ7ab"), "https://microvault.outray.app", sh)

	require.NoError(t, a.NotifyRepaymentMoreInfo("loan-1"))

	assert.Equal(t, moneyGramInfoURL, sh.gotURL, "dub shortens the destination, not our redirect")
	require.Len(t, notifier.moreInfo, 1)
	assert.Equal(t, "https://dub.sh/mg7x2", notifier.moreInfo[0].InteractiveURL)
}

// A shortener outage must not cost the borrower their instructions — the
// redirect resolves to the same place.
func TestNotifyRepaymentMoreInfo_FallsBackToTheRedirect(t *testing.T) {
	sh := &fakeShortener{err: errors.New("dub is down")}
	a, notifier := newTestRepaymentNotifierWith(t, loanWithMoreInfo("Xk9f2aQ7ab"), "https://microvault.outray.app", sh)

	require.NoError(t, a.NotifyRepaymentMoreInfo("loan-1"))

	require.Len(t, notifier.moreInfo, 1)
	assert.Equal(t, "https://microvault.outray.app/r/Xk9f2aQ7ab", notifier.moreInfo[0].InteractiveURL)
}
