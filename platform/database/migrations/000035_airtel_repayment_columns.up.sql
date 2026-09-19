-- Airtel Money repayment state on loans, mirroring the M-Pesa trio added in
-- 000032. repayment_provider already exists and gains a third value.
--
-- The column names differ from M-Pesa's in a way that matters. M-Pesa's
-- checkout id is minted by Daraja and handed back in the response; Airtel's
-- transaction id is minted by us and sent, which makes it the idempotency
-- key as well as the enquiry key. Airtel's own receipt arrives later and only
-- on success, so it is a separate, nullable column.

ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS repayment_airtel_txn_id varchar(64),
    ADD COLUMN IF NOT EXISTS repayment_airtel_money_id varchar(64),
    ADD COLUMN IF NOT EXISTS repayment_airtel_attempts int NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS uq_loans_repayment_airtel_txn_id
    ON loans (repayment_airtel_txn_id)
    WHERE repayment_airtel_txn_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_loans_repayment_airtel_money_id
    ON loans (repayment_airtel_money_id)
    WHERE repayment_airtel_money_id IS NOT NULL;

-- The Airtel enquiry poller's due set, mirroring idx_loans_repayment_stk_due.
CREATE INDEX IF NOT EXISTS idx_loans_repayment_airtel_due
    ON loans (repayment_next_poll_at)
    WHERE deleted_at IS NULL
      AND repayment_status = 'initiated'
      AND repayment_provider = 'airtel'
      AND repayment_airtel_txn_id IS NOT NULL;
