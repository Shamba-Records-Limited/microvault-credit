-- Retry state for the treasury-to-vault repay of an unwound disbursement.
--
-- vault_repay_status gains two values alongside success and failed:
-- "pending" is an atomic claim held while the on-chain call is in flight, and
-- "unknown" marks a call whose outcome could not be confirmed, which must be
-- verified on-chain before anything retries it.
--
-- vault_repay_amount_stroops fixes the amount owed at the first attempt, so a
-- MoneyGram refund that returned less than the principal is retried for what
-- came back rather than the principal. NULL means the principal.

ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS vault_repay_attempts int NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS vault_repay_amount_stroops bigint,
    ADD COLUMN IF NOT EXISTS vault_repay_attempted_at timestamptz;

-- The reconciler's due set.
CREATE INDEX IF NOT EXISTS idx_loans_vault_repay_due
    ON loans (vault_repay_attempted_at)
    WHERE deleted_at IS NULL
      AND vault_repay_tx_hash IS NULL
      AND vault_repay_status IN ('failed', 'pending');
