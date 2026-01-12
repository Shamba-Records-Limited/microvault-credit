-- Create credit_scoring_factors table
CREATE TABLE IF NOT EXISTS credit_scoring_factors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    factor_name VARCHAR(100) UNIQUE NOT NULL,
    factor_description TEXT,
    weight_bps INT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    calculation_method VARCHAR(100),
    min_data_required INT NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by UUID,
    updated_by UUID
);

CREATE INDEX idx_credit_scoring_factors_is_active ON credit_scoring_factors(is_active);

-- Create risk_tier_configs table
CREATE TABLE IF NOT EXISTS risk_tier_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tier_name VARCHAR(50) UNIQUE NOT NULL,
    min_score INT NOT NULL,
    max_score INT NOT NULL,
    tier_order INT UNIQUE NOT NULL,
    description TEXT,
    color_code VARCHAR(20),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by UUID,
    updated_by UUID
);

CREATE INDEX idx_risk_tier_configs_is_active ON risk_tier_configs(is_active);

-- Create loan_limit_configs table
CREATE TABLE IF NOT EXISTS loan_limit_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    risk_tier VARCHAR(50) UNIQUE NOT NULL,
    min_loan_amount BIGINT NOT NULL,
    max_loan_amount BIGINT NOT NULL,
    income_multiplier_bps INT NOT NULL,
    max_concurrent_loans INT NOT NULL,
    max_loan_duration_days INT NOT NULL,
    interest_rate_bps INT NOT NULL,
    late_fee_bps INT,
    default_penalty_bps INT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by UUID,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_by UUID
);

CREATE INDEX idx_loan_limit_configs_is_active ON loan_limit_configs(is_active);

-- Create global_lending_limits table
CREATE TABLE IF NOT EXISTS global_lending_limits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_key VARCHAR(100) UNIQUE NOT NULL,
    config_value VARCHAR(255) NOT NULL,
    value_type VARCHAR(20) NOT NULL,
    description TEXT,
    category VARCHAR(50),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by UUID,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_by UUID
);

CREATE INDEX idx_global_lending_limits_value_type ON global_lending_limits(value_type);
CREATE INDEX idx_global_lending_limits_category ON global_lending_limits(category);
CREATE INDEX idx_global_lending_limits_is_active ON global_lending_limits(is_active);

-- Create credit_config_audit_logs table
CREATE TABLE IF NOT EXISTS credit_config_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_table VARCHAR(100) NOT NULL,
    config_id UUID NOT NULL,
    action VARCHAR(20) NOT NULL,
    old_values JSONB,
    new_values JSONB,
    changed_by UUID,
    change_reason TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_credit_config_audit_logs_config_table ON credit_config_audit_logs(config_table);
CREATE INDEX idx_credit_config_audit_logs_config_id ON credit_config_audit_logs(config_id);
CREATE INDEX idx_credit_config_audit_logs_action ON credit_config_audit_logs(action);
CREATE INDEX idx_credit_config_audit_logs_created_at ON credit_config_audit_logs(created_at);
