DELETE FROM loan_products WHERE name = 'shamba_instant_loan';
ALTER TABLE loan_products DROP COLUMN IF EXISTS currency;
