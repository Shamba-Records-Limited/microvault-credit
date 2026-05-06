package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/moneygram"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mgpoller"
)

// MoneyGramPollerAdapter satisfies mgpoller.LoanFetcher and
// mgpoller.LoanRecorder by translating between the poller's projection
// (LoanRecord) and the credit service's loan model.
//
// LoanFetcher: scans `loans` rows where ramp_provider = "moneygram" and
// disbursement_status is in the active set, projecting each into a
// LoanRecord the poller can drive.
//
// LoanRecorder: writes the latest fields from a polled MG transaction
// onto the matching loan row. RecordSendUSDC is a no-op for now — the
// poller already uses MG's own stellar_transaction_id as the next-best
// idempotency marker. Add a dedicated column in a follow-up if the
// observation window proves problematic in production.
type MoneyGramPollerAdapter struct {
	loanSvc loan.Service
	logger  *slog.Logger
}

// NewMoneyGramPollerAdapter constructs the adapter. logger may be nil.
func NewMoneyGramPollerAdapter(loanSvc loan.Service, logger *slog.Logger) *MoneyGramPollerAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &MoneyGramPollerAdapter{
		loanSvc: loanSvc,
		logger:  logger.With("component", "moneygram_poller_adapter"),
	}
}

// Compile-time interface satisfaction.
var (
	_ mgpoller.LoanFetcher  = (*MoneyGramPollerAdapter)(nil)
	_ mgpoller.LoanRecorder = (*MoneyGramPollerAdapter)(nil)
)

// GetActiveMoneyGramLoans implements mgpoller.LoanFetcher.
func (a *MoneyGramPollerAdapter) GetActiveMoneyGramLoans(ctx context.Context, limit int) ([]mgpoller.LoanRecord, error) {
	loans, err := a.loanSvc.GetActiveByProvider(ctx, "moneygram", limit)
	if err != nil {
		return nil, fmt.Errorf("get active moneygram loans: %w", err)
	}

	out := make([]mgpoller.LoanRecord, 0, len(loans))
	for _, l := range loans {
		// Skip loans without the data the poller needs to drive state.
		// In normal operation these fields are populated by the MG branch
		// in loan_service_adapter; defensive null-checks here keep a
		// half-initialised row from crashing the poller loop.
		if l.RampRequestID == nil || *l.RampRequestID == "" {
			continue
		}
		rec := mgpoller.LoanRecord{
			LoanID:           l.ID,
			MoneyGramTxID:    *l.RampRequestID,
			PrincipalStroops: l.PrincipalAmount,
			UserID:           l.UserID,
			DisbursementStatus: strDerefAdapter(l.DisbursementStatus),
		}
		if l.RampSequenceID != nil {
			rec.SequenceID = *l.RampSequenceID
		} else {
			rec.SequenceID = l.ID
		}
		if l.RampChildAccountIndex != nil {
			rec.ChildAccountIndex = uint32(*l.RampChildAccountIndex)
		}
		if l.RequestedLocalAmount != nil {
			rec.RequestedLocalAmount = *l.RequestedLocalAmount
		}
		// PhoneNumber intentionally omitted — LoanResponse doesn't preload
		// the user record, and the disbursement adapter resolves SMS
		// recipients independently via NotifyDisbursementComplete /
		// NotifyDisbursementFailed.
		out = append(out, rec)
	}
	return out, nil
}

// RecordTransactionUpdate implements mgpoller.LoanRecorder. Persists the
// fields that change as MG advances state: amount_out / _asset / _fee
// (locked at pending_user_transfer_complete), external_transaction_id
// (the cash-pickup reference), and more_info_url.
func (a *MoneyGramPollerAdapter) RecordTransactionUpdate(ctx context.Context, loanID string, tx *moneygram.Transaction) error {
	if tx == nil {
		return nil
	}

	updateReq := loan.UpdateLoanRequest{}
	hasChanges := false

	// amount_out: parse decimal "1012.00" → cents int64 1012_00.
	if tx.AmountOut != "" {
		cents, err := parseDecimalToCents(tx.AmountOut)
		if err == nil && cents > 0 {
			updateReq.RampFiatAmount = &cents
			hasChanges = true
		}
	}

	// amount_out_asset: "iso4217:KES" → "KES".
	if tx.AmountOutAsset != "" {
		curr := stripISOPrefix(tx.AmountOutAsset)
		if curr != "" {
			updateReq.RampFiatCurr = &curr
			hasChanges = true
		}
	}

	// amount_fee: parse decimal → local-currency cents.
	if tx.AmountFee != "" {
		cents, err := parseDecimalToCents(tx.AmountFee)
		if err == nil && cents > 0 {
			updateReq.RampFeeLocal = &cents
			hasChanges = true
		}
	}

	if tx.ExternalTransactionID != "" {
		ref := tx.ExternalTransactionID
		updateReq.RampExternalRef = &ref
		hasChanges = true
	}

	if tx.MoreInfoURL != "" {
		url := tx.MoreInfoURL
		updateReq.RampMoreInfoURL = &url
		hasChanges = true
	}

	// withdraw_memo / withdraw_memo_type: persisted on first observation so
	// the Stellar ingest worker can match inbound USDC refunds back to the
	// originating loan. MG sends refunds with the same memo it issued at
	// SEP-24 init. Idempotent: re-asserting the same value is harmless.
	if tx.WithdrawMemo != "" {
		memo := tx.WithdrawMemo
		updateReq.RampWithdrawMemo = &memo
		hasChanges = true
	}
	if tx.WithdrawMemoType != "" {
		memoType := tx.WithdrawMemoType
		updateReq.RampWithdrawMemoType = &memoType
		hasChanges = true
	}

	if !hasChanges {
		return nil
	}

	if _, err := a.loanSvc.Update(ctx, loanID, updateReq); err != nil {
		return fmt.Errorf("record moneygram transaction update: %w", err)
	}
	return nil
}

// RecordSendUSDC implements mgpoller.LoanRecorder. No persistent column
// for the off-ramp USDC tx hash exists today; the poller relies on MG's
// stellar_transaction_id observation for idempotency. A dedicated column
// (e.g. ramp_settlement_tx_hash) can be added in a follow-up if needed.
func (a *MoneyGramPollerAdapter) RecordSendUSDC(_ context.Context, loanID, txHash string) error {
	a.logger.Info("USDC sent to MoneyGram (idempotency via tx.stellar_transaction_id)",
		"loan_id", loanID, "stellar_tx_hash", txHash)
	return nil
}

// parseDecimalToCents converts "1012.00" → 101200 cents. Returns 0 on
// malformed input; callers should treat 0 as "skip the field".
func parseDecimalToCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return 0, err
	}
	return int64(f * 100), nil
}

// stripISOPrefix turns "iso4217:KES" into "KES". Falls through unchanged
// if the prefix isn't there.
func stripISOPrefix(asset string) string {
	if i := strings.Index(asset, ":"); i >= 0 {
		return asset[i+1:]
	}
	return asset
}

// strDerefAdapter avoids name collision with strDeref in loan_service_adapter.go.
func strDerefAdapter(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
