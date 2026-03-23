ALTER TABLE loans ADD COLUMN IF NOT EXISTS vault_repay_tx_hash VARCHAR(64);
CREATE INDEX IF NOT EXISTS idx_loans_vault_repay_tx_hash ON loans (vault_repay_tx_hash);
