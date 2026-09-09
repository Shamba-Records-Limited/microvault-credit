DROP INDEX IF EXISTS idx_loans_repayment_next_poll_at;
DROP INDEX IF EXISTS idx_loans_repayment_open;
DROP INDEX IF EXISTS idx_loans_repayment_mg_tx_id;

ALTER TABLE loans
    DROP COLUMN IF EXISTS repayment_status,
    DROP COLUMN IF EXISTS repayment_payoff_stroops,
    DROP COLUMN IF EXISTS repayment_locked_at,
    DROP COLUMN IF EXISTS repayment_expires_at,
    DROP COLUMN IF EXISTS repayment_mg_tx_id,
    DROP COLUMN IF EXISTS repayment_next_poll_at;
