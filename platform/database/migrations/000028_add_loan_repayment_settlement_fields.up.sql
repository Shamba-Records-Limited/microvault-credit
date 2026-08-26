-- Two columns the deposit driver needs that migration 000026 did not
-- anticipate.
--
-- repayment_reminder_sent_at marks the single pre-expiry reminder SMS. It is
-- written before the send, so a failing SMS provider is not retried on every
-- poll tick for the rest of the window. This is the same reasoning that gave
-- ramp_pickup_ready_at its own column: it records a notification, not a
-- movement of money, so nothing else on the row can stand in for it.
--
-- repayment_vault_tx_hash is the treasury-to-vault repay_for transaction. It
-- cannot share vault_repay_tx_hash, which means "the disbursement was unwound"
-- and would be overwritten by a borrower settling their debt. Without it the
-- reconciliation loop has no cheap way to tell a loan whose vault leg landed
-- from one whose leg is still owed, and the only record of the hash would be
-- the transactions row.
ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS repayment_reminder_sent_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS repayment_vault_tx_hash VARCHAR(64);

-- Replace idx_loans_repayment_open with an index that matches the query that
-- actually exists.
--
-- 000026 built idx_loans_repayment_open as (repayment_status,
-- repayment_expires_at), on the assumption that expiry would be its own sweep.
-- It is not: the deposit driver reads repayment_expires_at off the row it is
-- already polling and expires it in place, so nothing ever filters or orders on
-- that column and the second half of the index serves nothing.
--
-- The one query that does exist asks for live repayments whose next poll is
-- due, ordered by that time. NULLS FIRST is not cosmetic: a repayment that has
-- never been polled has a NULL next_poll_at and is the most urgent row in the
-- set, which is also why the partial index from 000026 on
-- repayment_next_poll_at IS NOT NULL could not serve it either. Matching the
-- query's ordering lets one index satisfy both the filter and the sort.
--
-- Same partial predicate, so this is a narrowing of two indexes into one, not
-- an addition.
DROP INDEX IF EXISTS idx_loans_repayment_open;
DROP INDEX IF EXISTS idx_loans_repayment_next_poll_at;

CREATE INDEX IF NOT EXISTS idx_loans_repayment_due
    ON loans (repayment_next_poll_at NULLS FIRST)
    WHERE repayment_status IN ('initiated', 'funds_received');
