DROP INDEX IF EXISTS idx_loans_ramp_external_ref;

ALTER TABLE loans DROP COLUMN IF EXISTS requested_local_amount;
ALTER TABLE loans DROP COLUMN IF EXISTS entry_buffer_pct;
ALTER TABLE loans DROP COLUMN IF EXISTS entry_rate_source;
ALTER TABLE loans DROP COLUMN IF EXISTS entry_rate_used;

ALTER TABLE loans DROP COLUMN IF EXISTS ramp_child_account_index;
ALTER TABLE loans DROP COLUMN IF EXISTS ramp_more_info_url;
ALTER TABLE loans DROP COLUMN IF EXISTS ramp_external_ref;
ALTER TABLE loans DROP COLUMN IF EXISTS ramp_interactive_url;
