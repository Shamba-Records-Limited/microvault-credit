-- Drop foreign key constraint first
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS fk_transactions_loan_id;

-- Drop tables in reverse order
DROP TABLE IF EXISTS repayments;
DROP TABLE IF EXISTS credit_scores;
DROP TABLE IF EXISTS loans;
DROP TABLE IF EXISTS loan_products;
