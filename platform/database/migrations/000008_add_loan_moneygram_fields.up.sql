-- Add MoneyGram cash-pickup identifier fields and FX audit fields to loans.
--
-- Identifier fields support the SEP-24 lifecycle: the interactive webview
-- URL is delivered to the user via SMS; ramp_external_ref is the cash-pickup
-- reference number returned post-completion; ramp_more_info_url is MG's
-- support deep-link for the transaction; ramp_child_account_index is the
-- per-user Stellar derivation index used to re-derive the SEP-10 child memo
-- on poller restart.
--
-- FX audit fields capture the rate the loan was quoted at, the source label
-- (moneygram_fx_rate / yellowcard_fallback / cached_primary / cached_fallback),
-- the entry buffer percentage applied, and the user's originally requested
-- local amount — used by the poller's drift detection.

ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_interactive_url     TEXT;
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_external_ref        VARCHAR(100);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_more_info_url       TEXT;
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_child_account_index BIGINT;

ALTER TABLE loans ADD COLUMN IF NOT EXISTS entry_rate_used          NUMERIC(20, 8);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS entry_rate_source        VARCHAR(40);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS entry_buffer_pct         NUMERIC(6, 4);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS requested_local_amount   BIGINT;

-- Partial index for support-call lookups: cash-pickup refs are typically
-- queried by the support team given a ticket. Indexing only non-null rows
-- keeps the index small since most loans (mobile-money) won't have one.
CREATE INDEX IF NOT EXISTS idx_loans_ramp_external_ref
    ON loans(ramp_external_ref)
    WHERE ramp_external_ref IS NOT NULL;
