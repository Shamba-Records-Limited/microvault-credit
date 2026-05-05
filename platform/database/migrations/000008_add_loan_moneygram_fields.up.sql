-- Add MoneyGram cash-pickup off-ramp fields to loans table.
-- See internal-docs/moneygram-integration.md §13 for the persistence model.
-- No new table — MG state denormalises onto loans.ramp_* like YellowCard's does.

-- MoneyGram-specific identifiers populated on InitiateOffRamp + the poller.
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_interactive_url     TEXT;
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_external_ref        TEXT;
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_more_info_url       TEXT;
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_child_account_index INTEGER;

-- FX rate audit trail captured at off-ramp initiation. Nullable because YC
-- direct settlement disbursements don't pre-quote a rate (it's locked at
-- submission time inside YC's API). MG cash pickup must populate these so
-- ops can reconcile slippage between expected and locked payout.
ALTER TABLE loans ADD COLUMN IF NOT EXISTS entry_rate_used      NUMERIC(20,6);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS entry_rate_source    VARCHAR(40);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS entry_buffer_pct     NUMERIC(6,4);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS requested_local_amount NUMERIC(20,2);

-- The cash pickup reference is the lookup key when a user calls support
-- ("My reference is 12345678"). Indexed for fast retrieval; lookups are
-- idempotent because MG references are globally unique within their system.
CREATE INDEX IF NOT EXISTS idx_loans_ramp_external_ref ON loans (ramp_external_ref)
    WHERE ramp_external_ref IS NOT NULL;
