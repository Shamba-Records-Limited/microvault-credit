-- Add the SEP-24 withdraw memo and memo type that MoneyGram returns on the
-- transaction object. These are the values the treasury attaches to the USDC
-- payment sent into MG's anchor account; on refund, MG sends the USDC back
-- with the same memo so the ingest worker can match the inbound payment to
-- the originating loan.
--
-- ramp_withdraw_memo_type is typically "id" (numeric memo) but SEP-24 allows
-- "text" and "hash" — record what MG specifies so the ingest worker can
-- format the comparison correctly.

ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_withdraw_memo      VARCHAR(64);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_withdraw_memo_type VARCHAR(10);

-- Partial index unblocks the refund-by-memo lookup path. Only cash-pickup
-- loans populate this column, so the index stays small.
CREATE INDEX IF NOT EXISTS idx_loans_ramp_withdraw_memo
    ON loans(ramp_withdraw_memo)
    WHERE ramp_withdraw_memo IS NOT NULL;
