-- Revert the ramp short-code column from loans.

DROP INDEX IF EXISTS idx_loans_ramp_short_code;

ALTER TABLE loans DROP COLUMN IF EXISTS ramp_short_code;
