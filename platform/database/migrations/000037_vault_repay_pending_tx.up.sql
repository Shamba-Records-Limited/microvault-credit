-- The signed repay transaction, recorded before it is submitted.
--
-- A repay whose submission was never confirmed (poll timeout, transport error,
-- or a process that died mid-submit) can be settled from the ledger once the
-- transaction's validity window has closed: it either landed or it never can.
-- vault_repay_pending_tx_hash is what to look up; vault_repay_tx_expires_at is
-- the end of that window. Both are cleared when the outcome is recorded.

ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS vault_repay_pending_tx_hash varchar(64),
    ADD COLUMN IF NOT EXISTS vault_repay_tx_expires_at timestamptz;

-- The reconciler's resolution set.
CREATE INDEX IF NOT EXISTS idx_loans_vault_repay_resolvable
    ON loans (vault_repay_tx_expires_at)
    WHERE deleted_at IS NULL
      AND vault_repay_tx_hash IS NULL
      AND vault_repay_pending_tx_hash IS NOT NULL;
