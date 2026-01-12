-- Create loan_products table
CREATE TABLE IF NOT EXISTS loan_products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    interest_rate_bps INT NOT NULL,
    interest_type VARCHAR(20) NOT NULL DEFAULT 'simple',
    origination_fee_bps INT,
    min_amount BIGINT NOT NULL,
    max_amount BIGINT NOT NULL,
    min_duration_days INT NOT NULL,
    max_duration_days INT NOT NULL,
    allowed_repayment_schedules JSONB,
    max_credit_multiplier_bps INT NOT NULL,
    requires_collateral BOOLEAN NOT NULL DEFAULT FALSE,
    collateral_bps INT,
    priority_order INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

CREATE INDEX idx_loan_products_is_active ON loan_products(is_active);
CREATE INDEX idx_loan_products_deleted_at ON loan_products(deleted_at);

-- Create loans table
CREATE TABLE IF NOT EXISTS loans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    loan_number VARCHAR(50) UNIQUE,
    user_id UUID NOT NULL,
    account_id UUID NOT NULL,
    product_id UUID,
    principal_amount BIGINT NOT NULL,
    principal_asset VARCHAR(20) NOT NULL,
    interest_rate_bps INT NOT NULL,
    interest_amount BIGINT,
    origination_fee BIGINT,
    origination_fee_bps INT,
    total_amount BIGINT,
    duration_days INT NOT NULL,
    repayment_schedule VARCHAR(20) NOT NULL,
    due_date TIMESTAMP,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    approved_at TIMESTAMP,
    approved_by UUID,
    disbursed_at TIMESTAMP,
    repaid_at TIMESTAMP,
    defaulted_at TIMESTAMP,
    vault_tx_hash VARCHAR(64),
    vault_tx_status VARCHAR(20),
    ramp_provider VARCHAR(50),
    ramp_request_id VARCHAR(100),
    ramp_fiat_amount BIGINT,
    ramp_fiat_currency VARCHAR(10),
    momo_provider VARCHAR(50),
    momo_transaction_id VARCHAR(100),
    momo_status VARCHAR(20),
    disbursement_rate_bps BIGINT,
    disbursement_amount_kes BIGINT,
    repayment_amount_kes BIGINT,
    conversion_spread_bps INT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    FOREIGN KEY (product_id) REFERENCES loan_products(id) ON DELETE SET NULL
);

CREATE INDEX idx_loans_user_id ON loans(user_id);
CREATE INDEX idx_loans_account_id ON loans(account_id);
CREATE INDEX idx_loans_product_id ON loans(product_id);
CREATE INDEX idx_loans_principal_asset ON loans(principal_asset);
CREATE INDEX idx_loans_due_date ON loans(due_date);
CREATE INDEX idx_loans_status ON loans(status);
CREATE INDEX idx_loans_disbursed_at ON loans(disbursed_at);
CREATE INDEX idx_loans_vault_tx_hash ON loans(vault_tx_hash);
CREATE INDEX idx_loans_ramp_request_id ON loans(ramp_request_id);
CREATE INDEX idx_loans_momo_transaction_id ON loans(momo_transaction_id);
CREATE INDEX idx_loans_deleted_at ON loans(deleted_at);

-- Create credit_scores table
CREATE TABLE IF NOT EXISTS credit_scores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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

-- Create repayments table
CREATE TABLE IF NOT EXISTS repayments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    loan_id UUID NOT NULL,
    user_id UUID NOT NULL,
    installment_number INT NOT NULL,
    due_date TIMESTAMP NOT NULL,
    amount_due BIGINT NOT NULL,
    amount_paid BIGINT NOT NULL DEFAULT 0,
    paid_at TIMESTAMP,
    payment_method VARCHAR(50),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    late_fee BIGINT NOT NULL DEFAULT 0,
    transaction_id UUID,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    FOREIGN KEY (loan_id) REFERENCES loans(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (transaction_id) REFERENCES transactions(id) ON DELETE SET NULL
);

CREATE INDEX idx_repayments_loan_id ON repayments(loan_id);
CREATE INDEX idx_repayments_user_id ON repayments(user_id);
CREATE INDEX idx_repayments_due_date ON repayments(due_date);
CREATE INDEX idx_repayments_status ON repayments(status);
CREATE INDEX idx_repayments_transaction_id ON repayments(transaction_id);
CREATE INDEX idx_repayments_deleted_at ON repayments(deleted_at);

-- Add foreign key to transactions table for loan_id
ALTER TABLE transactions ADD CONSTRAINT fk_transactions_loan_id
    FOREIGN KEY (loan_id) REFERENCES loans(id) ON DELETE SET NULL;
