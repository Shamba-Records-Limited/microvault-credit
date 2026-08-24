-- The vault leg needs a durable attempt count.
--
-- When repay_for fails, the borrower's USDC is already on the treasury and the
-- borrower has already been told their repayment landed. The deposit driver
-- retries on its active backoff for as long as MoneyGram keeps reporting
-- completed, which until now had no ceiling and no escalation: a permanently
-- failing vault leg would retry every two minutes forever, and the only signal
-- would be a log line nobody is watching.
--
-- The count lives on the row rather than in memory for the same reason the
-- poll schedule does. A process restart must not reset it — that would turn a
-- loan that has failed fifty times into one that has failed zero, and the
-- escalation would never fire.
--
-- It also does double duty as the escalation marker. Attempts only ever
-- increment, so the tick where the count crosses the configured ceiling is a
-- single event, and alerting exactly on that crossing needs no second column
-- and cannot spam ops.
ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS repayment_vault_attempts INTEGER NOT NULL DEFAULT 0;
