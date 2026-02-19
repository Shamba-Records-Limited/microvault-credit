-- Revert disbursement tracking, borrow index, and fee columns from loans table.

DROP INDEX IF EXISTS idx_loans_ramp_sequence_id;
DROP INDEX IF EXISTS idx_loans_disbursement_status;

ALTER TABLE loans DROP COLUMN IF EXISTS ramp_fee_local;
ALTER TABLE loans DROP COLUMN IF EXISTS ramp_fee_usd;
ALTER TABLE loans DROP COLUMN IF EXISTS borrow_index;
ALTER TABLE loans DROP COLUMN IF EXISTS ramp_sequence_id;
ALTER TABLE loans DROP COLUMN IF EXISTS disbursement_status;
ALTER TABLE loans DROP COLUMN IF EXISTS settlement_method;
