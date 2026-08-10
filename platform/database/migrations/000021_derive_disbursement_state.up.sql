-- Replace disbursement_status with the loan's own status plus two markers.
--
-- The column carried two unrelated jobs. Most of its values restate what is
-- already knowable: "completed" is loans.status='disbursed', "failed" is
-- 'offramp_failed', "refund_received" is 'cancelled'. Keeping a second copy
-- meant they could disagree, and they did — MoneyGram wrote "completed" while
-- the reader compared against YellowCard's "complete", so no cash-pickup loan
-- ever left disbursing (see migration 000020's code change).
--
-- The two values that are NOT derivable get purpose-built columns:
--
-- ramp_refund_declared_at — the anchor said "refunded" but the USDC has not
-- been verified on-ledger. No transaction row exists yet by design: the refund
-- row is written only after PaymentsTo confirms the payment, which is what
-- stops an unverified claim from driving a vault repay. A timestamp also makes
-- "declared days ago, still unverified" an ops query rather than an alert
-- someone had to be watching for, and it survives restarts, which the poller's
-- in-memory wait counter did not.
--
-- ramp_pickup_ready_at — the cash is collectable and the borrower has been told.
-- This is an SMS side effect, so nothing in the ledger records it. It is
-- written before the send, so a failed SMS does not re-send every 30s tick;
-- that was previously disbursement_status='processing'.
ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS ramp_refund_declared_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS ramp_pickup_ready_at TIMESTAMPTZ;

-- Backfill from the column about to be dropped, so in-flight loans keep their
-- place in the state machine across the deploy.
UPDATE loans SET ramp_refund_declared_at = NOW()
    WHERE disbursement_status = 'refund_pending' AND ramp_refund_declared_at IS NULL;

UPDATE loans SET ramp_pickup_ready_at = NOW()
    WHERE disbursement_status = 'processing' AND ramp_pickup_ready_at IS NULL;

-- Terminal outcomes recorded before the status mapping was fixed: the loan
-- status was left at 'disbursing', which kept settled loans inside
-- GetActiveLoans and counted them against the borrower.
UPDATE loans SET status = 'disbursed'
    WHERE disbursement_status IN ('completed', 'complete') AND status = 'disbursing';

UPDATE loans SET status = 'offramp_failed'
    WHERE disbursement_status = 'failed' AND status = 'disbursing';

UPDATE loans SET status = 'cancelled'
    WHERE disbursement_status = 'refund_received' AND status = 'disbursing';

DROP INDEX IF EXISTS idx_loans_disbursement_status;

ALTER TABLE loans DROP COLUMN IF EXISTS disbursement_status;

-- The refund sweep and the MoneyGram poller both scan for loans still in
-- flight, which is now a status filter rather than a disbursement_status one.
CREATE INDEX IF NOT EXISTS idx_loans_ramp_provider_status
    ON loans (ramp_provider, status)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_loans_refund_declared
    ON loans (ramp_refund_declared_at)
    WHERE ramp_refund_declared_at IS NOT NULL;
