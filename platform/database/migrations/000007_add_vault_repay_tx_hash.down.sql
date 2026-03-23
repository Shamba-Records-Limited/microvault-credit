DROP INDEX IF EXISTS idx_loans_vault_repay_tx_hash;
ALTER TABLE loans DROP COLUMN IF EXISTS vault_repay_tx_hash;
