package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/stellaranchor"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mgpoller"
)

// Compile-time checks.
var (
	_ mgpoller.LoanFetcher  = (*MoneyGramPollerAdapter)(nil)
	_ mgpoller.LoanRecorder = (*MoneyGramPollerAdapter)(nil)
)

// MoneyGramPollerAdapter bridges microvault-credit's loan repository to the
// generic mgpoller in microvault. mgpoller defines the state machine and
// HTTP loop; this adapter supplies the persistence half — projecting loan
// rows into mgpoller.LoanRecord and writing the per-tick deltas back to the
// loan via the loan service's typed update path.
type MoneyGramPollerAdapter struct {
	repo    repository.LoanRepository
	loanSvc loan.Service
	logger  *slog.Logger
}

// NewMoneyGramPollerAdapter builds the adapter. repo and loanSvc are
// required; logger may be nil.
func NewMoneyGramPollerAdapter(
	repo repository.LoanRepository,
	loanSvc loan.Service,
	logger *slog.Logger,
) (*MoneyGramPollerAdapter, error) {
	if repo == nil {
		return nil, fmt.Errorf("moneygram poller adapter: repo is required")
	}
	if loanSvc == nil {
		return nil, fmt.Errorf("moneygram poller adapter: loan service is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &MoneyGramPollerAdapter{
		repo:    repo,
		loanSvc: loanSvc,
		logger:  logger.With("component", "mgpoller_adapter"),
	}, nil
}

// GetActiveMoneyGramLoans returns the loans the mgpoller should evaluate
// this tick: ramp_provider="moneygram" with non-terminal disbursement_status.
func (a *MoneyGramPollerAdapter) GetActiveMoneyGramLoans(ctx context.Context, limit int) ([]mgpoller.LoanRecord, error) {
	loans, err := a.repo.GetActiveMoneyGramLoans(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]mgpoller.LoanRecord, 0, len(loans))
	for _, l := range loans {
		if l == nil {
			continue
		}
		out = append(out, projectLoanRecord(l))
	}
	return out, nil
}

// RecordTransactionUpdate persists the SEP-24 transaction fields onto the
// loan: amount_out + currency, fee, MG external reference, MoreInfoURL, and
// the withdraw memo MG hands back (needed by the Stellar ingest worker for
// refund matching). All fields are optional in the SEP-24 response — we
// only set those that arrived non-empty.
func (a *MoneyGramPollerAdapter) RecordTransactionUpdate(ctx context.Context, loanID string, tx *stellaranchor.Transaction) error {
	if tx == nil {
		return fmt.Errorf("moneygram poller adapter: nil transaction for loan %s", loanID)
	}

	req := loan.UpdateLoanRequest{}
	any := false

	var grossCents int64
	var grossKnown bool
	if v := strings.TrimSpace(tx.AmountOut); v != "" {
		cents, ok := decimalToCents(v)
		if ok {
			req.RampFiatAmount = &cents
			grossCents = cents
			grossKnown = true
			any = true
		}
	}
	if v := strings.TrimPrefix(strings.TrimSpace(tx.AmountOutAsset), "iso4217:"); v != "" {
		req.RampFiatCurr = &v
		any = true
	}
	var feeCents int64
	if v := strings.TrimSpace(tx.AmountFee); v != "" {
		cents, ok := decimalToCents(v)
		if ok {
			req.RampFeeLocal = &cents
			feeCents = cents
			any = true
		}
	}
	// MG's SEP-24 amount_out is the value the user receives, already net of
	// amount_fee — so it equals delivered_amount_kes. (Unlike YC's
	// converted_amount, which is gross and requires subtracting ramp_fee_local
	// — handled in DisbursementStatusAdapter.recordDeliveredAmount.)
	_ = feeCents
	if grossKnown {
		delivered := grossCents
		req.DeliveredAmtKES = &delivered
	}
	if v := strings.TrimSpace(tx.ExternalTransactionID); v != "" {
		req.RampExternalRef = &v
		any = true
	}
	if v := strings.TrimSpace(tx.MoreInfoURL); v != "" {
		req.RampMoreInfoURL = &v
		any = true
	}
	if v := strings.TrimSpace(tx.WithdrawMemo); v != "" {
		req.RampWithdrawMemo = &v
		any = true
	}
	if v := strings.TrimSpace(tx.WithdrawMemoType); v != "" {
		req.RampWithdrawMemoType = &v
		any = true
	}

	if !any {
		return nil
	}

	if _, err := a.loanSvc.Update(ctx, loanID, req); err != nil {
		return fmt.Errorf("moneygram poller adapter: update loan %s: %w", loanID, err)
	}
	return nil
}

// RecordSendUSDC logs the treasury to MG anchor tx hash. There is no
// dedicated column on loans for this yet, and the mgpoller already uses
// MG's own tx.stellar_transaction_id as the idempotency marker, so logging
// here is sufficient for audit until a column is added.
func (a *MoneyGramPollerAdapter) RecordSendUSDC(_ context.Context, loanID string, txHash string) error {
	a.logger.Info("treasury to MoneyGram USDC send recorded",
		"loan_id", loanID, "tx_hash", txHash)
	return nil
}

// projectLoanRecord maps a Loan row into the mgpoller projection. Pointer
// fields default to their zero values when nil — the poller skips records
// without a MoneyGramTxID, which catches the case where Initiate hasn't
// populated RampRequestID yet.
func projectLoanRecord(l *models.Loan) mgpoller.LoanRecord {
	rec := mgpoller.LoanRecord{
		LoanID:           l.ID,
		UserID:           l.UserID,
		PrincipalStroops: l.PrincipalAmount,
	}
	if l.RampSequenceID != nil {
		rec.SequenceID = *l.RampSequenceID
	}
	if l.RampRequestID != nil {
		rec.MoneyGramTxID = *l.RampRequestID
	}
	if l.RampChildAccountIndex != nil {
		// Stored as int64 to fit the column, fits a uint32 by construction.
		rec.ChildAccountIndex = uint32(*l.RampChildAccountIndex)
	}
	if l.RequestedLocalAmount != nil {
		// Persisted as cents; the poller's drift check works in major units.
		rec.RequestedLocalAmount = float64(*l.RequestedLocalAmount) / 100.0
	}
	if l.DisbursementStatus != nil {
		rec.DisbursementStatus = *l.DisbursementStatus
	}
	if l.User != nil {
		rec.PhoneNumber = l.User.MobileNumber
	}
	return rec
}

// decimalToCents converts a SEP-24 decimal string (e.g. "1250.00") to an
// int64 in cents. Returns (0, false) on malformed input — callers should
// skip the field rather than write a zero.
func decimalToCents(s string) (int64, bool) {
	var v float64
	if _, err := fmt.Sscanf(s, "%f", &v); err != nil {
		return 0, false
	}
	if v < 0 {
		return 0, false
	}
	return int64(v * 100), true
}
