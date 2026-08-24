-- Structural recreate only, no seed data. The id column carries no
-- gen_random_uuid() default: migration 000004 removed it and the model set ids
-- via BeforeCreate, so this matches the schema as it stood before the drop.
CREATE TABLE IF NOT EXISTS repayments (
    id UUID PRIMARY KEY,
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
