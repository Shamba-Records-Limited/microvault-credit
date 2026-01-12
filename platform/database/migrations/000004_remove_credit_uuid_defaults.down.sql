-- Restore DEFAULT gen_random_uuid() to credit tables

-- Loan tables
ALTER TABLE loan_products ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE loans ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE credit_scores ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE repayments ALTER COLUMN id SET DEFAULT gen_random_uuid();

-- Document tables
ALTER TABLE user_documents ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE transactions_extracted ALTER COLUMN id SET DEFAULT gen_random_uuid();

-- Analysis tables
ALTER TABLE cashflow_analyses ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE farm_records ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE credit_factors ALTER COLUMN id SET DEFAULT gen_random_uuid();

-- Config tables
ALTER TABLE credit_scoring_factors ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE risk_tier_configs ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE loan_limit_configs ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE global_lending_limits ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE credit_config_audit_logs ALTER COLUMN id SET DEFAULT gen_random_uuid();
