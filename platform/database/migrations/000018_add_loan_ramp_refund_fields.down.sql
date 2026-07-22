DROP INDEX IF EXISTS idx_loans_ramp_refund_shortfall;

ALTER TABLE loans
    DROP COLUMN IF EXISTS ramp_refund_tx_hash,
    DROP COLUMN IF EXISTS ramp_refund_amount,
    DROP COLUMN IF EXISTS ramp_refund_shortfall,
    DROP COLUMN IF EXISTS ramp_refunded_at;
