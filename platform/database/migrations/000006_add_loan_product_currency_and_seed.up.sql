-- Add currency column to loan_products.
-- min_amount and max_amount are now denominated in this fiat currency (stored as cents).
ALTER TABLE loan_products ADD COLUMN currency VARCHAR(10) NOT NULL DEFAULT 'KES';

-- Seed the default loan product used by the USSD instant-loan flow.
-- Amounts in fiat cents: min = 50000 (KES 500), max = 300000 (KES 3,000).
-- Fixed 30-day term, lump-sum repayment.
-- interest_rate_bps = 500 (5 %) is a fallback; the vault's dynamic APR takes precedence at runtime.
INSERT INTO loan_products (
    id, name, description, interest_rate_bps, interest_type,
    min_amount, max_amount, currency,
    min_duration_days, max_duration_days,
    allowed_repayment_schedules, max_credit_multiplier_bps,
    priority_order, is_active
) VALUES (
    gen_random_uuid(),
    'shamba_instant_loan',
    'Instant mobile money loan for Shamba Records users',
    500, 'simple',
    50000, 300000, 'KES',
    30, 30,
    '["lump_sum"]', 10000,
    1, TRUE
) ON CONFLICT (name) DO NOTHING;
