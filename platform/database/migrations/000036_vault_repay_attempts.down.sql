DROP INDEX IF EXISTS idx_loans_vault_repay_due;

ALTER TABLE loans
    DROP COLUMN IF EXISTS vault_repay_attempted_at,
    DROP COLUMN IF EXISTS vault_repay_amount_stroops,
    DROP COLUMN IF EXISTS vault_repay_attempts;
