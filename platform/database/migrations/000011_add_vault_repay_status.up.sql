-- Add vault_repay_status to mirror vault_tx_status.
--
-- vault_repay_tx_hash already records the on-chain hash on a successful
-- repay, but failures leave both columns NULL, indistinguishable from
-- "never attempted". This column surfaces failed repays so ops can query
-- and retry without grepping logs.
--
-- Values: "success" | "failed" (NULL when no repay has been attempted, e.g.
-- direct settlement where USDC moved to YC and no repay is owed).

ALTER TABLE loans ADD COLUMN IF NOT EXISTS vault_repay_status VARCHAR(20);
CREATE INDEX IF NOT EXISTS idx_loans_vault_repay_status ON loans(vault_repay_status);
