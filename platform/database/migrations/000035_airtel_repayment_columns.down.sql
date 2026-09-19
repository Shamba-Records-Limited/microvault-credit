DROP INDEX IF EXISTS idx_loans_repayment_airtel_due;
DROP INDEX IF EXISTS uq_loans_repayment_airtel_money_id;
DROP INDEX IF EXISTS uq_loans_repayment_airtel_txn_id;

ALTER TABLE loans
    DROP COLUMN IF EXISTS repayment_airtel_attempts,
    DROP COLUMN IF EXISTS repayment_airtel_money_id,
    DROP COLUMN IF EXISTS repayment_airtel_txn_id;
