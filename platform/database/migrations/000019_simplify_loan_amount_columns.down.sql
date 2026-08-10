-- Renames reverse exactly, including the 10^4 rescale. The dropped columns
-- come back empty: interest_amount and total_amount were derived from an APR
-- and duration that are still present, but quoted_repayment_amount_kes also
-- folded in a conversion rate captured at quote time and nothing else recorded
-- it. The momo_* columns never held anything to restore.
ALTER TABLE loans
    ALTER COLUMN disbursement_rate TYPE BIGINT
    USING (round(disbursement_rate * 10000))::bigint;
ALTER TABLE loans RENAME COLUMN disbursement_rate TO disbursement_rate_bps;

ALTER TABLE loans RENAME COLUMN delivered_amount_local TO delivered_amount_kes;
ALTER TABLE loans RENAME COLUMN entry_rate_buffered TO entry_rate_used;
ALTER TABLE loans RENAME COLUMN vault_apr_bps TO interest_rate_bps;

ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS momo_provider VARCHAR(50),
    ADD COLUMN IF NOT EXISTS momo_transaction_id VARCHAR(100),
    ADD COLUMN IF NOT EXISTS momo_status VARCHAR(20),
    ADD COLUMN IF NOT EXISTS interest_amount BIGINT,
    ADD COLUMN IF NOT EXISTS total_amount BIGINT,
    ADD COLUMN IF NOT EXISTS quoted_repayment_amount_kes BIGINT,
    ADD COLUMN IF NOT EXISTS quoted_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_loans_momo_transaction_id ON loans(momo_transaction_id);
