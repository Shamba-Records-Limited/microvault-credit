-- Structural recreate only, no seed data. The id columns carry no
-- gen_random_uuid() default: migration 000004 removed those and the models set
-- ids via BeforeCreate, so this matches the schema as it stood before the drop.
-- Parents (credit_scores, user_documents) are created before the tables that
-- reference them.

CREATE TABLE IF NOT EXISTS credit_scores (
    id UUID PRIMARY KEY,
    user_id UUID UNIQUE NOT NULL,
    score INT NOT NULL,
    score_version VARCHAR(20) NOT NULL,
    total_loans INT NOT NULL DEFAULT 0,
    successful_repayments INT NOT NULL DEFAULT 0,
    defaults INT NOT NULL DEFAULT 0,
    current_outstanding BIGINT NOT NULL DEFAULT 0,
    days_overdue INT NOT NULL DEFAULT 0,
    max_loan_amount BIGINT,
    max_concurrent_loans INT NOT NULL DEFAULT 1,
    calculated_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_credit_scores_score ON credit_scores(score);
CREATE INDEX idx_credit_scores_expires_at ON credit_scores(expires_at);
CREATE INDEX idx_credit_scores_deleted_at ON credit_scores(deleted_at);

CREATE TABLE IF NOT EXISTS user_documents (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    document_type VARCHAR(50) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    file_size INT NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    storage_path VARCHAR(500) NOT NULL,
    upload_date TIMESTAMP NOT NULL,
    processing_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    parsed_data JSONB,
    error_message TEXT,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_user_documents_user_id ON user_documents(user_id);
CREATE INDEX idx_user_documents_document_type ON user_documents(document_type);
CREATE INDEX idx_user_documents_processing_status ON user_documents(processing_status);

CREATE TABLE IF NOT EXISTS transactions_extracted (
    id UUID PRIMARY KEY,
    document_id UUID NOT NULL,
    user_id UUID NOT NULL,
    transaction_date TIMESTAMP NOT NULL,
    transaction_type VARCHAR(20) NOT NULL,
    amount BIGINT NOT NULL,
    balance_after BIGINT,
    counterparty VARCHAR(255),
    description TEXT,
    category VARCHAR(50),
    source VARCHAR(20) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    FOREIGN KEY (document_id) REFERENCES user_documents(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_transactions_extracted_document_id ON transactions_extracted(document_id);
CREATE INDEX idx_transactions_extracted_user_id ON transactions_extracted(user_id);
CREATE INDEX idx_transactions_extracted_transaction_date ON transactions_extracted(transaction_date);
CREATE INDEX idx_transactions_extracted_transaction_type ON transactions_extracted(transaction_type);
CREATE INDEX idx_transactions_extracted_category ON transactions_extracted(category);
CREATE INDEX idx_transactions_extracted_source ON transactions_extracted(source);
CREATE INDEX idx_transactions_extracted_deleted_at ON transactions_extracted(deleted_at);

CREATE TABLE IF NOT EXISTS cashflow_analyses (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    analysis_period_start TIMESTAMP NOT NULL,
    analysis_period_end TIMESTAMP NOT NULL,
    total_income BIGINT NOT NULL DEFAULT 0,
    total_expenses BIGINT NOT NULL DEFAULT 0,
    net_cashflow BIGINT NOT NULL DEFAULT 0,
    avg_monthly_income BIGINT NOT NULL DEFAULT 0,
    avg_monthly_expenses BIGINT NOT NULL DEFAULT 0,
    income_stability_score INT NOT NULL DEFAULT 0,
    expense_regularity_score INT NOT NULL DEFAULT 0,
    savings_rate_bps INT NOT NULL DEFAULT 0,
    transaction_count INT NOT NULL DEFAULT 0,
    unique_income_sources INT NOT NULL DEFAULT 0,
    income_sources JSONB,
    expense_categories JSONB,
    seasonal_pattern JSONB,
    risk_flags JSONB,
    calculated_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_cashflow_analyses_user_id ON cashflow_analyses(user_id);
CREATE INDEX idx_cashflow_analyses_period_start ON cashflow_analyses(analysis_period_start);
CREATE INDEX idx_cashflow_analyses_period_end ON cashflow_analyses(analysis_period_end);
CREATE INDEX idx_cashflow_analyses_deleted_at ON cashflow_analyses(deleted_at);

CREATE TABLE IF NOT EXISTS farm_records (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    record_type VARCHAR(50) NOT NULL,
    record_date TIMESTAMP NOT NULL,
    crop_type VARCHAR(100),
    quantity BIGINT,
    unit VARCHAR(50),
    amount BIGINT,
    description TEXT,
    document_id UUID,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (document_id) REFERENCES user_documents(id) ON DELETE SET NULL
);
CREATE INDEX idx_farm_records_user_id ON farm_records(user_id);
CREATE INDEX idx_farm_records_record_type ON farm_records(record_type);
CREATE INDEX idx_farm_records_record_date ON farm_records(record_date);
CREATE INDEX idx_farm_records_document_id ON farm_records(document_id);

CREATE TABLE IF NOT EXISTS credit_factors (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    credit_score_id UUID,
    factor_name VARCHAR(100) NOT NULL,
    factor_value_bps INT NOT NULL,
    factor_weight_bps INT NOT NULL,
    weighted_score_bps INT NOT NULL,
    calculation_method VARCHAR(100),
    data_source VARCHAR(50),
    calculated_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (credit_score_id) REFERENCES credit_scores(id) ON DELETE SET NULL
);
CREATE INDEX idx_credit_factors_user_id ON credit_factors(user_id);
CREATE INDEX idx_credit_factors_credit_score_id ON credit_factors(credit_score_id);
CREATE INDEX idx_credit_factors_factor_name ON credit_factors(factor_name);
CREATE INDEX idx_credit_factors_calculated_at ON credit_factors(calculated_at);

CREATE TABLE IF NOT EXISTS credit_scoring_factors (
    id UUID PRIMARY KEY,
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

CREATE TABLE IF NOT EXISTS risk_tier_configs (
    id UUID PRIMARY KEY,
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

CREATE TABLE IF NOT EXISTS credit_config_audit_logs (
    id UUID PRIMARY KEY,
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
