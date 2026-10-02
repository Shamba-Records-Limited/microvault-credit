DROP INDEX IF EXISTS idx_loans_vault_repay_resolvable;

ALTER TABLE loans
    DROP COLUMN IF EXISTS vault_repay_tx_expires_at,
    DROP COLUMN IF EXISTS vault_repay_pending_tx_hash;
