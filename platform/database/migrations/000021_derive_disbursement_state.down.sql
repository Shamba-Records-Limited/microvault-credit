-- Restores the column and reconstructs what can be reconstructed. Values that
-- were only ever derivable come back derived; anything else is lost.
ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS disbursement_status VARCHAR(30);

UPDATE loans SET disbursement_status = CASE
    WHEN status = 'cancelled'                     THEN 'refund_received'
    WHEN status = 'offramp_failed'                THEN 'failed'
    WHEN status = 'disbursed'                     THEN 'completed'
    WHEN ramp_refund_declared_at IS NOT NULL      THEN 'refund_pending'
    WHEN ramp_pickup_ready_at IS NOT NULL         THEN 'processing'
    WHEN ramp_provider = 'moneygram'              THEN 'mg_initiated'
    ELSE 'pending'
END
WHERE ramp_provider IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_loans_disbursement_status ON loans(disbursement_status);

DROP INDEX IF EXISTS idx_loans_refund_declared;
DROP INDEX IF EXISTS idx_loans_ramp_provider_status;

ALTER TABLE loans
    DROP COLUMN IF EXISTS ramp_pickup_ready_at,
    DROP COLUMN IF EXISTS ramp_refund_declared_at;
