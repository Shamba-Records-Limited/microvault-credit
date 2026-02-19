-- Add disbursement tracking, borrow index, and fee columns to loans table.
-- These columns support: settlement method tracking, off-ramp sequence IDs,
-- on-chain borrow index for per-borrower debt calculation, and fee capture.

ALTER TABLE loans ADD COLUMN IF NOT EXISTS settlement_method VARCHAR(20);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS disbursement_status VARCHAR(30);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_sequence_id VARCHAR(200);
ALTER TABLE loans ADD COLUMN IF NOT EXISTS borrow_index BIGINT;
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_fee_usd BIGINT;
ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_fee_local BIGINT;

-- Indexes for frequently queried columns
CREATE INDEX IF NOT EXISTS idx_loans_disbursement_status ON loans(disbursement_status);
CREATE INDEX IF NOT EXISTS idx_loans_ramp_sequence_id ON loans(ramp_sequence_id);
