ALTER TABLE loans DROP COLUMN IF EXISTS quoted_at;
ALTER TABLE loans RENAME COLUMN quoted_repayment_amount_kes TO repayment_amount_kes;
