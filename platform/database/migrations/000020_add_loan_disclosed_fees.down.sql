ALTER TABLE loans
    DROP COLUMN IF EXISTS telco_fee_usd,
    DROP COLUMN IF EXISTS telco_fee_local,
    DROP COLUMN IF EXISTS tax_usd,
    DROP COLUMN IF EXISTS tax_local;
