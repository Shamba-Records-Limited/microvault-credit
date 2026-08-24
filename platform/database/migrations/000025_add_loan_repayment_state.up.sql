-- Borrower-initiated repayment state, tracked separately from both
-- loans.status and the vault_repay_* columns.
--
-- vault_repay_tx_hash / vault_repay_status are NOT reused here. They mean
-- "a disbursement was unwound" — the treasury returning USDC the borrower
-- never received, after a failed off-ramp or an MG refund. A borrower paying
-- their loan off is the opposite movement and must not be able to overwrite
-- the record of the other.
--
-- loans.status is not reused either. The borrower-visible state and the
-- on-chain state are allowed to diverge for a window by design: cash lands on
-- the treasury and the borrower is told immediately, while the treasury-to-
-- vault leg may still be retrying. During that window repayment_status is
-- funds_received and loans.status is still disbursed. Only once the vault leg
-- confirms does the loan flip to repaid.
ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS repayment_status VARCHAR(20) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS repayment_payoff_stroops BIGINT,
    ADD COLUMN IF NOT EXISTS repayment_locked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS repayment_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS repayment_mg_tx_id VARCHAR(100),
    ADD COLUMN IF NOT EXISTS repayment_next_poll_at TIMESTAMPTZ;

-- The idempotency key for the cash rail. MoneyGram issues the transaction ID,
-- so replaying a completed deposit poll cannot produce a second vault_repay:
-- the second claim on the same MG transaction fails on this index rather than
-- on a check the poller has to remember to write.
CREATE UNIQUE INDEX IF NOT EXISTS idx_loans_repayment_mg_tx_id
    ON loans (repayment_mg_tx_id)
    WHERE repayment_mg_tx_id IS NOT NULL;

-- Covers both sweeps over open repayments: the expiry sweep (initiated past
-- repayment_expires_at) and the reconciliation loop (stuck at funds_received).
-- Partial, because 'none' is the overwhelming majority and every terminal
-- state is of no further interest to either sweep.
CREATE INDEX IF NOT EXISTS idx_loans_repayment_open
    ON loans (repayment_status, repayment_expires_at)
    WHERE repayment_status IN ('initiated', 'funds_received');

-- The deposit poller's schedule. Cadence lives in a column rather than an
-- in-memory ticker because the cash-in window is 4-5 days: a restart would
-- lose in-memory timers, and ticking MoneyGram every 30 seconds for three days
-- is tens of thousands of pointless requests.
CREATE INDEX IF NOT EXISTS idx_loans_repayment_next_poll_at
    ON loans (repayment_next_poll_at)
    WHERE repayment_next_poll_at IS NOT NULL;
