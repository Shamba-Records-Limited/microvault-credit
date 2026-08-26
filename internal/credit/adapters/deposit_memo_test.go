package adapters

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Shamba-Records-Limited/microvault/pkg/payment/stellaranchor"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
)

const (
	treasuryAddr = "GDLGMIMK7MM6YHJIPPRAVZSCFYJWE735GJ3BONBWROGW3LFGXSYL3RLB"
	authAddr     = "GCU7KVTYEPCUZOQPJ6RLNWW64FS3IBHGZTEFWERO7ZZVDQYHMG7W5B6M"
)

// The child memo is namespaced by the key it is seeded with — ChildAccountMemo
// hashes that key with the index — so initiation and polling must seed it from
// the same wallet.
//
// The poller seeds it from the anchor client's auth address. Initiation seeded
// it from the treasury, which is identical only because both default to
// TREASURY_SECRET_KEY. Setting MONEYGRAM_AUTH_SECRET to any other wallet put
// the deposit in a memo space the poller never queries: the borrower could pay
// and nothing would ever see it.
func TestChildMemoSeedsDivergeAcrossWallets(t *testing.T) {
	const idx = 7

	fromTreasury := stellaranchor.ChildAccountMemo(treasuryAddr, idx)
	fromAuth := stellaranchor.ChildAccountMemo(authAddr, idx)

	assert.NotEqual(t, fromTreasury, fromAuth,
		"different seeds must give different memos, which is why the seed has to be agreed")
}

// The adapter now holds the two addresses separately, because they answer
// different questions: where the money lands, and which memo namespace the
// session belongs to.
func TestAdapterKeepsDepositDestinationAndMemoSeedApart(t *testing.T) {
	a := &LoanServiceAdapter{
		repayTreasuryPubkey: treasuryAddr,
		repayAuthPubkey:     authAddr,
	}

	assert.Equal(t, treasuryAddr, a.repayTreasuryPubkey,
		"the deposit is credited to the treasury, which is the wallet the vault can spend from")
	assert.Equal(t, authAddr, a.repayAuthPubkey,
		"the memo follows the SEP-10 signer, which is what the poller derives from")
	assert.NotEqual(t, a.repayTreasuryPubkey, a.repayAuthPubkey)
}

func TestDepositMemoFor(t *testing.T) {
	ref := "LR-19F6C760B82-4bb8"
	withRef := &loan.LoanResponse{LoanReference: &ref}
	empty := ""

	assert.Equal(t, ref, depositMemoFor(withRef, "01a037a7-4614-7463-8db8-1c8d704fd4d3"),
		"the reference is what the borrower and support both quote")

	assert.Equal(t, "01a037a7-4614-7463-8db8-1c8d", depositMemoFor(&loan.LoanResponse{}, "01a037a7-4614-7463-8db8-1c8d704fd4d3"),
		"falls back to the loan ID, truncated to MEMO_TEXT's 28 bytes")

	assert.Equal(t, "01a037a7-4614-7463-8db8-1c8d", depositMemoFor(&loan.LoanResponse{LoanReference: &empty}, "01a037a7-4614-7463-8db8-1c8d704fd4d3"),
		"an empty reference is not a reference")
}

// MEMO_TEXT is 28 bytes. A longer value would be rejected by the network, and
// the payment the memo exists to identify would never be made.
func TestDepositMemoFor_NeverExceedsMemoTextLimit(t *testing.T) {
	long := "LR-THIS-REFERENCE-IS-FAR-TOO-LONG-TO-FIT-IN-A-TEXT-MEMO"
	got := depositMemoFor(&loan.LoanResponse{LoanReference: &long}, "loan-1")

	assert.LessOrEqual(t, len(got), 28)
	assert.Equal(t, long[:28], got)
}
