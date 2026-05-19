-- Revert the SEP-24 withdraw memo columns from loans.

DROP INDEX IF EXISTS idx_loans_ramp_withdraw_memo;

ALTER TABLE loans DROP COLUMN IF EXISTS ramp_withdraw_memo_type;
ALTER TABLE loans DROP COLUMN IF EXISTS ramp_withdraw_memo;
