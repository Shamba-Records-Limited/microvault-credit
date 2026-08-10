DROP INDEX IF EXISTS idx_loans_ramp_more_info_short_code;

ALTER TABLE loans
    DROP COLUMN IF EXISTS ramp_more_info_short_code;
