-- Create user_documents table
CREATE TABLE IF NOT EXISTS user_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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

-- Create transactions_extracted table
CREATE TABLE IF NOT EXISTS transactions_extracted (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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

-- Create cashflow_analyses table
CREATE TABLE IF NOT EXISTS cashflow_analyses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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

-- Create farm_records table
CREATE TABLE IF NOT EXISTS farm_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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

-- Create credit_factors table
CREATE TABLE IF NOT EXISTS credit_factors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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
