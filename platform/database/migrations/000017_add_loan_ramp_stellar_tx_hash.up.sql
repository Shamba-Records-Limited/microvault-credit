-- Records the treasury -> MoneyGram USDC payment hash. Acts as the local
-- idempotency marker for the cash-pickup poller: without it the poller relies
-- solely on MoneyGram echoing stellar_transaction_id, and re-sends the payment
-- on every 30s tick until MG catches up (observed: 15 duplicate sends).
ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS ramp_stellar_tx_hash VARCHAR(64);
