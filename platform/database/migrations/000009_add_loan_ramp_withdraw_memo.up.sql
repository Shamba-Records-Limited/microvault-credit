-- Persist MoneyGram's SEP-24 withdraw_memo on the loan row.
-- See docs/moneygram-integration.md §13 (refund handling).
--
-- MG supplies `withdraw_memo` on the SEP-24 transaction response. The poller
-- passes it as the Stellar payment memo when sending USDC to MG's anchor
-- account. On refund, MG sends USDC back to our treasury account using the
-- SAME memo — the Stellar ingest worker matches the inbound payment by this
-- memo to identify which loan the refund belongs to.
--
-- This is distinct from `ramp_child_account_index` (the SEP-10 child memo
-- derivation input) — see pkg/payment/moneygram/memo.go for the namespace
-- distinction.

ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_withdraw_memo      VARCHAR(64);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_withdraw_memo_type VARCHAR(16);

-- Partial index drives the refund matcher's lookup. Non-null only after the
-- poller has observed the SEP-24 transaction at least once (i.e. for active
-- and terminal MG cash-pickup loans).
CREATE INDEX IF NOT EXISTS idx_loans_ramp_withdraw_memo ON loans (ramp_withdraw_memo)
    WHERE ramp_withdraw_memo IS NOT NULL;
