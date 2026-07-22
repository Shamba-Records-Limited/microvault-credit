-- Records the settlement of a MoneyGram refund, which is what a borrower
-- cancelling in MG's own UI produces. The poller writes these before repaying
-- the vault, so a crash mid-repay still leaves evidence of what came back.
--
-- ramp_refund_shortfall is the stroops MG did not return (including its own
-- refund fee). Non-zero means the treasury absorbed the difference and the
-- loan needs a human to settle it, so it doubles as the flag ops queries on.
ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS ramp_refund_tx_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS ramp_refund_amount BIGINT,
    ADD COLUMN IF NOT EXISTS ramp_refund_shortfall BIGINT,
    ADD COLUMN IF NOT EXISTS ramp_refunded_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_loans_ramp_refund_shortfall
    ON loans (ramp_refund_shortfall)
    WHERE ramp_refund_shortfall IS NOT NULL AND ramp_refund_shortfall > 0;
