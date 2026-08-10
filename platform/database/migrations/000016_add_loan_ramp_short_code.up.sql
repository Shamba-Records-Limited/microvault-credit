-- Short, unguessable code mapping to a loan's MoneyGram interactive URL.
-- The full SEP-24 URL embeds a ~500-char JWT that overflows an SMS, so we SMS
-- a /r/{code} redirect and resolve the real URL here.

ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_short_code VARCHAR(24);

CREATE UNIQUE INDEX IF NOT EXISTS idx_loans_ramp_short_code
    ON loans(ramp_short_code)
    WHERE ramp_short_code IS NOT NULL;
