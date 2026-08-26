-- The borrower needs MoneyGram's deposit reference to pay at a counter, and
-- the SMS carrying it needs a marker of its own.
--
-- Testing found the gap: borrowers completed the webview and never received a
-- confirmation code, because nothing in the deposit flow sent one. The driver
-- now sends it when MoneyGram reports the borrower has committed, which is the
-- only point where a reference exists and the cash has not yet been handed
-- over.
--
-- Marked before the send, for the same reason repayment_reminder_sent_at is: a
-- failing SMS provider must not be retried on every poll tick for the rest of
-- the window. It records a notification rather than a movement of money, so
-- nothing else on the row can stand in for it.
ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS repayment_reference_sent_at TIMESTAMPTZ;
