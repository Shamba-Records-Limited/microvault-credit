ALTER TABLE loans
    ALTER COLUMN entry_buffer_bps TYPE NUMERIC(6,4)
    USING (entry_buffer_bps::numeric / 10000);
ALTER TABLE loans RENAME COLUMN entry_buffer_bps TO entry_buffer_pct;

ALTER TABLE loans
    ALTER COLUMN entry_rate_buffered TYPE NUMERIC(20,8)
    USING (entry_rate_buffered::numeric / 100000000);

ALTER TABLE loans
    ALTER COLUMN disbursement_rate TYPE NUMERIC(20,8)
    USING (disbursement_rate::numeric / 100000000);
