package repository

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	pkgErrors "github.com/Shamba-Records-Limited/microvault/pkg/errors"
)

// Common errors for LoanRepository
var (
	ErrLoanNotFound = errors.New("loan not found")
	// ErrLoanReferenceConflict is a loan_reference unique-index violation. The
	// reference generator retries on it rather than surfacing a 500 for what is
	// a rare collision in a 32^6 space.
	ErrLoanReferenceConflict          = errors.New("loan reference already exists")
	ErrFailedToCreateLoan             = errors.New("failed to create loan")
	ErrFailedToGetLoan                = errors.New("failed to get loan")
	ErrFailedToGetLoansByUserID       = errors.New("failed to get loans by user ID")
	ErrFailedToGetLoanByReference     = errors.New("failed to get loan by reference")
	ErrFailedToGetActiveLoans         = errors.New("failed to get active loans")
	ErrFailedToGetActiveLoansByStatus = errors.New("failed to get active loans by status")
	ErrFailedToUpdateLoan             = errors.New("failed to update loan")
	ErrFailedToRestoreLoan            = errors.New("failed to restore loan")
	ErrFailedToDeleteLoan             = errors.New("failed to delete loan")
	ErrFailedToDeleteLoansByUserID    = errors.New("failed to delete loans by user ID")
	ErrFailedToGetLoanBySequenceID    = errors.New("failed to get loan by sequence ID")
	ErrFailedToGetLoansByDisbStatus   = errors.New("failed to get loans by disbursement status")
	ErrFailedToGetLoanByWithdrawMemo  = errors.New("failed to get loan by ramp withdraw memo")
	ErrFailedToGetLoanByExternalRef   = errors.New("failed to get loan by ramp external ref")
	ErrFailedToGetLoanByShortCode     = errors.New("failed to get loan by ramp short code")
	ErrFailedToGetActiveMGLoans       = errors.New("failed to get active MoneyGram loans")
	ErrFailedToGetDueRepayments       = errors.New("failed to get due repayments")
	ErrFailedToGetActiveByProvider    = errors.New("failed to get active loans by provider")
)

// LoanRepository defines the interface for loanRespository data access.
type LoanRepository interface {
	// Create operations
	Create(ctx context.Context, loan *models.Loan) error

	// Read operations
	GetByID(ctx context.Context, id string) (*models.Loan, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.Loan, error)
	GetActiveLoans(ctx context.Context, limit, offset int) ([]*models.Loan, error)
	GetActiveLoansByStatus(ctx context.Context, status string, limit, offset int) ([]*models.Loan, error)
	GetBySequenceID(ctx context.Context, sequenceID string) (*models.Loan, error)
	// GetByAnyReference resolves a loan by either its current reference or the
	// legacy LR-... form preserved at migration time. This is the single lookup
	// C2B validation, C2B Hakikisha and the Pull reconciler share, so the
	// legacy path retires in one place.
	GetByAnyReference(ctx context.Context, reference string) (*models.Loan, error)
	// GetRefundDeclared returns loans whose anchor has declared a refund that
	// has not yet settled, oldest first. Scoped by provider because each
	// provider's refund is resolved through its own API.
	GetRefundDeclared(ctx context.Context, provider string, limit int) ([]*models.Loan, error)
	GetByRampWithdrawMemo(ctx context.Context, memo string) (*models.Loan, error)
	GetByRampExternalRef(ctx context.Context, ref string) (*models.Loan, error)
	GetByRampShortCode(ctx context.Context, code string) (*models.Loan, error)
	GetActiveMoneyGramLoans(ctx context.Context, limit int) ([]*models.Loan, error)

	// GetDueRepayments returns loans with a borrower repayment in flight whose
	// next poll is due. Preloads User and Account: the deposit driver needs the
	// phone number for SMS and the child account's public key to attribute the
	// vault repay.
	GetDueRepayments(ctx context.Context, limit int) ([]*models.Loan, error)

	// GetDueSTKRepayments returns M-Pesa Express repayments whose prompt may
	// have resolved and whose poll is due.
	GetDueSTKRepayments(ctx context.Context, limit int) ([]*models.Loan, error)

	// GetActiveByProvider returns loans where ramp_provider matches the given
	// provider and the loan has not reached a terminal status. Used by
	// provider-specific pollers to enumerate work.
	GetActiveByProvider(ctx context.Context, provider string, limit int, offset int) ([]*models.Loan, error)

	// GetActiveByUserAndProvider scopes the same query to a single user.
	// Used as the dedupe gate: a non-empty result means the user already has
	// an in-flight off-ramp for that provider and a new request should fail.
	GetActiveByUserAndProvider(ctx context.Context, userID, provider string) ([]*models.Loan, error)

	// Update operations
	Update(ctx context.Context, loan *models.Loan) error

	// UpdateFields writes only the supplied columns. Preferred over Update for
	// partial changes: Update rewrites every column, which on this table means
	// re-writing the ~1KB SEP-24 URLs and touching all 20 indexes for a
	// one-field change. Keys are validated against the same allow-list Update
	// uses; unknown columns are rejected.
	UpdateFields(ctx context.Context, id string, fields map[string]any) error
	Restore(ctx context.Context, id string) error

	// Delete operations
	Delete(ctx context.Context, id string) error
	DeleteByUserID(ctx context.Context, userID string) error
}

// loanRepository represents a repository for managing loans.
type loanRepository struct {
	db *gorm.DB
}

// NewLoanRepository creates a new instance of LoanRepository.
func NewLoanRepository(db *gorm.DB) (LoanRepository, error) {
	if db == nil {
		return nil, pkgErrors.ErrNilDB
	}
	return &loanRepository{db: db}, nil
}

// --- Create Operations ---

// Create creates a new loan record.
func (r *loanRepository) Create(ctx context.Context, loan *models.Loan) error {
	result := r.db.WithContext(ctx).Create(loan)
	if result.Error != nil {
		var pgErr *pgconn.PgError
		if errors.As(result.Error, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("%w: %v", ErrLoanReferenceConflict, result.Error)
		}
		log.Printf("Create: database error: %v", result.Error)
		return ErrFailedToCreateLoan
	}
	return nil
}

// --- Read Operations ---

// GetByID retrieves a loan record by its ID.
// GetByID loads one loan with its user.
func (r *loanRepository) GetByID(ctx context.Context, id string) (*models.Loan, error) {
	var loan models.Loan
	result := r.db.WithContext(ctx).
		Preload("User").
		Preload("Account").
		Where("id = ? AND deleted_at IS NULL", id).
		First(&loan)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("GetByID: database error: %v", result.Error)
		return nil, ErrFailedToGetLoan
	}
	return &loan, nil
}

// GetByUserID retrieves all loan records for a given user ID.
func (r *loanRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.Loan, error) {
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetByUserID: database error: %v", result.Error)
		return nil, ErrFailedToGetLoansByUserID
	}
	return loans, nil
}

// GetActiveLoans returns a list of active loans.
func (r *loanRepository) GetActiveLoans(ctx context.Context, limit, offset int) ([]*models.Loan, error) {
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Where("status IN ? AND deleted_at IS NULL",
			[]string{models.LoanStatusApproved, models.LoanStatusDisbursing, models.LoanStatusDisbursed}).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetActiveLoans: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveLoans
	}
	return loans, nil
}

// GetActiveLoansByStatus returns a list of active loans by status.
func (r *loanRepository) GetActiveLoansByStatus(ctx context.Context, status string, limit, offset int) ([]*models.Loan, error) {
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Where("status IN ? AND deleted_at IS NULL", status).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetActiveLoansByStatus: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveLoansByStatus
	}
	return loans, nil
}

// inFlightLoanStatuses are the statuses a loan holds while its payout is still
// running. Replaces a disbursement_status filter that listed pending and
// processing — values MoneyGram never wrote, so MG loans matched neither and
// the per-user dedupe gate let a borrower open concurrent cash pickups.
var inFlightLoanStatuses = []string{models.LoanStatusApproved, models.LoanStatusDisbursing}

func (r *loanRepository) GetActiveByProvider(ctx context.Context, provider string, limit, offset int) ([]*models.Loan, error) {
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Where("ramp_provider = ? AND status IN ? AND deleted_at IS NULL",
			provider, inFlightLoanStatuses).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetActiveByProvider: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveByProvider
	}
	return loans, nil
}

func (r *loanRepository) GetActiveByUserAndProvider(ctx context.Context, userID, provider string) ([]*models.Loan, error) {
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND ramp_provider = ? AND status IN ? AND deleted_at IS NULL",
			userID, provider, inFlightLoanStatuses).
		Order("created_at DESC").
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetActiveByUserAndProvider: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveByProvider
	}
	return loans, nil
}

// GetBySequenceID retrieves a loan by its YellowCard ramp sequence ID.
// Closes caveat A: handles suffix variants if exact match is not found (e.g. direct-to-fiat _fiat pivot).
func (r *loanRepository) GetBySequenceID(ctx context.Context, sequenceID string) (*models.Loan, error) {
	var loan models.Loan
	// 1. Try exact match first
	result := r.db.WithContext(ctx).
		Preload("User").
		Where("ramp_sequence_id = ? AND deleted_at IS NULL", sequenceID).
		First(&loan)
	if result.Error == nil {
		return &loan, nil
	}

	// 2. Fallback to matching suffix variants if record not found
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		var alternativeID string
		if strings.HasSuffix(sequenceID, "_fiat") {
			alternativeID = strings.TrimSuffix(sequenceID, "_fiat")
		} else {
			alternativeID = sequenceID + "_fiat"
		}

		resultAlt := r.db.WithContext(ctx).
			Preload("User").
			Where("ramp_sequence_id = ? AND deleted_at IS NULL", alternativeID).
			First(&loan)
		if resultAlt.Error == nil {
			return &loan, nil
		}
		if errors.Is(resultAlt.Error, gorm.ErrRecordNotFound) {
			return nil, ErrLoanNotFound
		}
		log.Printf("GetBySequenceID (alt): database error: %v", resultAlt.Error)
		return nil, ErrFailedToGetLoanBySequenceID
	}

	log.Printf("GetBySequenceID: database error: %v", result.Error)
	return nil, ErrFailedToGetLoanBySequenceID
}

// GetRefundDeclared implements LoanRepository.
//
// Callers driving a provider-specific state machine must pass their provider:
// a refund is resolved through the declaring provider's API, so an unscoped
// query hands one provider's poller another provider's loans.
func (r *loanRepository) GetRefundDeclared(ctx context.Context, provider string, limit int) ([]*models.Loan, error) {
	var loans []*models.Loan
	query := r.db.WithContext(ctx).
		Preload("User").
		Where("ramp_refund_declared_at IS NOT NULL AND status IN ? AND deleted_at IS NULL",
			inFlightLoanStatuses)
	if provider != "" {
		query = query.Where("ramp_provider = ?", provider)
	}
	result := query.
		Order("ramp_refund_declared_at ASC").
		Limit(limit).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetRefundDeclared: database error: %v", result.Error)
		return nil, ErrFailedToGetLoansByDisbStatus
	}
	return loans, nil
}

// GetByRampWithdrawMemo retrieves a loan by its SEP-24 withdraw memo. Used by
// the Stellar ingest worker to match an inbound refund USDC payment back to
// the originating loan.
func (r *loanRepository) GetByRampWithdrawMemo(ctx context.Context, memo string) (*models.Loan, error) {
	var loan models.Loan
	result := r.db.WithContext(ctx).
		Preload("User").
		Where("ramp_withdraw_memo = ? AND deleted_at IS NULL", memo).
		First(&loan)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("GetByRampWithdrawMemo: database error: %v", result.Error)
		return nil, ErrFailedToGetLoanByWithdrawMemo
	}
	return &loan, nil
}

// GetByRampExternalRef retrieves a loan by its MoneyGram cash-pickup
// reference number. Used by the support team to look up a loan from a
// customer-quoted reference.
func (r *loanRepository) GetByRampExternalRef(ctx context.Context, ref string) (*models.Loan, error) {
	var loan models.Loan
	result := r.db.WithContext(ctx).
		Preload("User").
		Where("ramp_external_ref = ? AND deleted_at IS NULL", ref).
		First(&loan)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("GetByRampExternalRef: database error: %v", result.Error)
		return nil, ErrFailedToGetLoanByExternalRef
	}
	return &loan, nil
}

// GetByRampShortCode resolves the loan behind a /r/{code} SMS redirect.
// GetByAnyReference resolves a loan by loan_reference first, then
// legacy_loan_reference. Callers must validate the reference shape before
// calling — a garbage reference should be rejected upstream without a query.
func (r *loanRepository) GetByAnyReference(ctx context.Context, reference string) (*models.Loan, error) {
	var loan models.Loan
	result := r.db.WithContext(ctx).
		Where("loan_reference = ? AND deleted_at IS NULL", reference).
		First(&loan)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		result = r.db.WithContext(ctx).
			Where("legacy_loan_reference = ? AND deleted_at IS NULL", reference).
			First(&loan)
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("GetByAnyReference: database error: %v", result.Error)
		return nil, ErrFailedToGetLoanByReference
	}
	return &loan, nil
}

func (r *loanRepository) GetByRampShortCode(ctx context.Context, code string) (*models.Loan, error) {
	var loan models.Loan
	result := r.db.WithContext(ctx).
		Where("(ramp_short_code = ? OR ramp_more_info_short_code = ?) AND deleted_at IS NULL", code, code).
		First(&loan)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("GetByRampShortCode: database error: %v", result.Error)
		return nil, ErrFailedToGetLoanByShortCode
	}
	return &loan, nil
}

// GetActiveMoneyGramLoans returns loans currently being driven by the MG
// poller — ramp_provider="moneygram" that have not reached a terminal status.
// Preloads User so the poller has the phone number for drift / failure SMS
// without a second round-trip.
func (r *loanRepository) GetActiveMoneyGramLoans(ctx context.Context, limit int) ([]*models.Loan, error) {
	if limit <= 0 {
		limit = 100
	}
	// A declared-but-unsettled refund stays in this set: the loan is not
	// finished, it is awaiting inbound USDC, and the MG poller is the only
	// thing that will settle it. It is still LoanStatusDisbursing until the
	// refund is verified, so no special case is needed.
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Preload("User").
		Where("ramp_provider = ? AND status IN ? AND deleted_at IS NULL",
			"moneygram", inFlightLoanStatuses).
		Order("created_at ASC").
		Limit(limit).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetActiveMoneyGramLoans: database error: %v", result.Error)
		return nil, ErrFailedToGetActiveMGLoans
	}
	return loans, nil
}

// openRepaymentStatuses are the repayment states the deposit driver owns.
// Matches the predicate of idx_loans_repayment_open; keep the two in step.
var openRepaymentStatuses = []string{
	models.LoanRepaymentStatusInitiated,
	models.LoanRepaymentStatusFundsReceived,
}

// GetDueRepayments returns repayments the deposit driver should evaluate this
// tick. Scoped to the MoneyGram rail: M-Pesa-initiated loans are driven by the
// STK fetcher, never this one.
func (r *loanRepository) GetDueRepayments(ctx context.Context, limit int) ([]*models.Loan, error) {
	if limit <= 0 {
		limit = 100
	}
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Preload("User").
		Preload("Account").
		Where("repayment_status IN ? AND deleted_at IS NULL", openRepaymentStatuses).
		Where("repayment_provider = ?", models.LoanRepaymentProviderMoneyGram).
		Where("repayment_next_poll_at IS NULL OR repayment_next_poll_at <= ?", time.Now()).
		Order("repayment_next_poll_at ASC NULLS FIRST").
		Limit(limit).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetDueRepayments: database error: %v", result.Error)
		return nil, ErrFailedToGetDueRepayments
	}
	return loans, nil
}

// GetDueSTKRepayments returns M-Pesa Express repayments whose prompt may have
// resolved and whose poll is due.
func (r *loanRepository) GetDueSTKRepayments(ctx context.Context, limit int) ([]*models.Loan, error) {
	if limit <= 0 {
		limit = 100
	}
	var loans []*models.Loan
	result := r.db.WithContext(ctx).
		Where("repayment_status = ?", models.LoanRepaymentStatusInitiated).
		Where("repayment_provider = ?", models.LoanRepaymentProviderMpesa).
		Where("repayment_mpesa_checkout_id IS NOT NULL AND deleted_at IS NULL").
		Where("repayment_next_poll_at IS NULL OR repayment_next_poll_at <= ?", time.Now()).
		Order("repayment_next_poll_at ASC NULLS FIRST").
		Limit(limit).
		Find(&loans)
	if result.Error != nil {
		log.Printf("GetDueSTKRepayments: database error: %v", result.Error)
		return nil, ErrFailedToGetDueRepayments
	}
	return loans, nil
}

// --- Update Operations ---

// loanUpdateMap is the single source of truth for which loan columns may be
// written, and how each maps onto the model. Both Update (full rewrite) and
// UpdateFields (partial) derive from it, so the allow-list cannot drift out of
// sync with the writer — an omission here has silently dropped writes before.
func loanUpdateMap(loan *models.Loan) map[string]interface{} {
	return map[string]interface{}{
		"status":                 loan.Status,
		"approved_at":            loan.ApprovedAt,
		"approved_by":            loan.ApprovedBy,
		"disbursed_at":           loan.DisbursedAt,
		"repaid_at":              loan.RepaidAt,
		"defaulted_at":           loan.DefaultedAt,
		"vault_tx_hash":          loan.VaultTxHash,
		"vault_tx_status":        loan.VaultTxStatus,
		"vault_repay_tx_hash":    loan.VaultRepayTxHash,
		"vault_repay_status":     loan.VaultRepayStatus,
		"ramp_provider":          loan.RampProvider,
		"ramp_request_id":        loan.RampRequestID,
		"ramp_fiat_amount":       loan.RampFiatAmount,
		"ramp_fiat_currency":     loan.RampFiatCurr,
		"settlement_method":      loan.SettlementMethod,
		"ramp_sequence_id":       loan.RampSequenceID,
		"origination_fee":        loan.OriginationFee,
		"origination_fee_bps":    loan.OriginationFeeBps,
		"disbursement_rate":      loan.DisbursementRate,
		"delivered_amount_local": loan.DeliveredAmountLocal,
		"conversion_spread_bps":  loan.ConversionSpreadBps,
		"borrow_index":           loan.BorrowIndex,
		"service_fee_usd":        loan.ServiceFeeUSD,
		"service_fee_local":      loan.ServiceFeeLocal,
		"partner_fee_usd":        loan.PartnerFeeUSD,
		"partner_fee_local":      loan.PartnerFeeLocal,
		"telco_fee_usd":          loan.TelcoFeeUSD,
		"telco_fee_local":        loan.TelcoFeeLocal,
		"tax_usd":                loan.TaxUSD,
		"tax_local":              loan.TaxLocal,

		"ramp_interactive_url":       loan.RampInteractiveURL,
		"ramp_short_code":            loan.RampShortCode,
		"ramp_short_code_expires_at": loan.RampShortCodeExpiresAt,
		"ramp_more_info_short_code":  loan.RampMoreInfoShortCode,
		"ramp_external_ref":          loan.RampExternalRef,
		"ramp_more_info_url":         loan.RampMoreInfoURL,
		"ramp_child_account_index":   loan.RampChildAccountIndex,
		"ramp_stellar_tx_hash":       loan.RampStellarTxHash,
		"entry_rate_buffered":        loan.EntryRateBuffered,
		"entry_rate_source":          loan.EntryRateSource,
		"entry_buffer_bps":           loan.EntryBufferBps,
		"requested_local_amount":     loan.RequestedLocalAmount,
		"ramp_withdraw_memo":         loan.RampWithdrawMemo,
		"ramp_withdraw_memo_type":    loan.RampWithdrawMemoType,
		"ramp_refund_declared_at":    loan.RampRefundDeclaredAt,
		"ramp_pickup_ready_at":       loan.RampPickupReadyAt,
		"ramp_refund_tx_hash":        loan.RampRefundTxHash,
		"ramp_refund_amount":         loan.RampRefundAmount,
		"ramp_refund_shortfall":      loan.RampRefundShortfall,
		"ramp_refunded_at":           loan.RampRefundedAt,

		"repayment_status":            loan.RepaymentStatus,
		"repayment_payoff_stroops":    loan.RepaymentPayoffStroops,
		"repayment_locked_at":         loan.RepaymentLockedAt,
		"repayment_expires_at":        loan.RepaymentExpiresAt,
		"repayment_mg_tx_id":          loan.RepaymentMGTxID,
		"repayment_next_poll_at":      loan.RepaymentNextPollAt,
		"repayment_reminder_sent_at":  loan.RepaymentReminderSentAt,
		"repayment_vault_tx_hash":     loan.RepaymentVaultTxHash,
		"repayment_vault_attempts":    loan.RepaymentVaultAttempts,
		"repayment_reference_sent_at": loan.RepaymentReferenceSentAt,
		"repayment_provider":          loan.RepaymentProvider,
		"repayment_mpesa_checkout_id": loan.RepaymentMpesaCheckoutID,
		"repayment_mpesa_trans_id":    loan.RepaymentMpesaTransID,
		"repayment_stk_attempts":      loan.RepaymentSTKAttempts,
	}
}

// loanUpdatableColumns is the set of column names loanUpdateMap can write.
var loanUpdatableColumns = func() map[string]bool {
	cols := make(map[string]bool)
	for k := range loanUpdateMap(&models.Loan{}) {
		cols[k] = true
	}
	return cols
}()

// IsUpdatableColumn reports whether col may be written via UpdateFields. Exposed
// so the service layer's partial-update mapping can be test-guarded against
// drift from this allow-list.
func IsUpdatableColumn(col string) bool {
	return loanUpdatableColumns[col]
}

// Update rewrites every updatable column from the model. Prefer UpdateFields
// for partial changes.
func (r *loanRepository) Update(ctx context.Context, loan *models.Loan) error {
	fields := loanUpdateMap(loan)
	fields["updated_at"] = time.Now()
	return r.updateColumns(ctx, loan.ID, fields)
}

// UpdateFields writes only the supplied columns. See the interface docs.
func (r *loanRepository) UpdateFields(ctx context.Context, id string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(fields)+1)
	for k, v := range fields {
		if !loanUpdatableColumns[k] {
			log.Printf("UpdateFields: rejected non-updatable column %q", k)
			return ErrFailedToUpdateLoan
		}
		out[k] = v
	}
	out["updated_at"] = time.Now()
	return r.updateColumns(ctx, id, out)
}

// updateColumns applies a pre-validated column map to one loan row.
func (r *loanRepository) updateColumns(ctx context.Context, id string, fields map[string]interface{}) error {
	result := r.db.WithContext(ctx).
		Model(&models.Loan{}).
		Omit(clause.Associations).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(fields)
	if result.Error != nil {
		log.Printf("Update: database error: %v", result.Error)
		return ErrFailedToUpdateLoan
	}
	if result.RowsAffected == 0 {
		return ErrLoanNotFound
	}
	return nil
}

// Restore restores a loan by ID.
func (r *loanRepository) Restore(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.Loan{}).
		Where("id = ?", id).
		Update("deleted_at", nil)
	if result.RowsAffected == 0 {
		return ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("Restore: database error: %v", result.Error)
		return ErrFailedToRestoreLoan
	}
	return nil
}

// --- Delete operations ---

// Delete deletes a loan by ID.
func (r *loanRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Loan{})
	if result.RowsAffected == 0 {
		return ErrLoanNotFound
	}
	if result.Error != nil {
		log.Printf("Delete: database error: %v", result.Error)
		return ErrFailedToDeleteLoan
	}
	return nil
}

// DeleteByUserID deletes a loan by user ID.
func (r *loanRepository) DeleteByUserID(ctx context.Context, userID string) error {
	result := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&models.Loan{})
	if result.Error != nil {
		log.Printf("DeleteByUserID: database error: %v", result.Error)
		return ErrFailedToDeleteLoansByUserID
	}
	return nil
}
