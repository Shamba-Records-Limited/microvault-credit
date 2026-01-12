-- Remove DEFAULT gen_random_uuid() from credit tables
-- IDs are now generated at application layer using UUIDv7

-- Loan tables
ALTER TABLE loan_products ALTER COLUMN id DROP DEFAULT;
ALTER TABLE loans ALTER COLUMN id DROP DEFAULT;
ALTER TABLE credit_scores ALTER COLUMN id DROP DEFAULT;
ALTER TABLE repayments ALTER COLUMN id DROP DEFAULT;

-- Document tables
ALTER TABLE user_documents ALTER COLUMN id DROP DEFAULT;
ALTER TABLE transactions_extracted ALTER COLUMN id DROP DEFAULT;

-- Analysis tables
ALTER TABLE cashflow_analyses ALTER COLUMN id DROP DEFAULT;
ALTER TABLE farm_records ALTER COLUMN id DROP DEFAULT;
ALTER TABLE credit_factors ALTER COLUMN id DROP DEFAULT;

-- Config tables
ALTER TABLE credit_scoring_factors ALTER COLUMN id DROP DEFAULT;
ALTER TABLE risk_tier_configs ALTER COLUMN id DROP DEFAULT;
ALTER TABLE loan_limit_configs ALTER COLUMN id DROP DEFAULT;
ALTER TABLE global_lending_limits ALTER COLUMN id DROP DEFAULT;
ALTER TABLE credit_config_audit_logs ALTER COLUMN id DROP DEFAULT;
