DROP INDEX IF EXISTS idx_loans_repayment_due;

-- Restore the two indexes 000026 created.
CREATE INDEX IF NOT EXISTS idx_loans_repayment_open
    ON loans (repayment_status, repayment_expires_at)
    WHERE repayment_status IN ('initiated', 'funds_received');

CREATE INDEX IF NOT EXISTS idx_loans_repayment_next_poll_at
    ON loans (repayment_next_poll_at)
    WHERE repayment_next_poll_at IS NOT NULL;

ALTER TABLE loans
    DROP COLUMN IF EXISTS repayment_reminder_sent_at,
    DROP COLUMN IF EXISTS repayment_vault_tx_hash;
