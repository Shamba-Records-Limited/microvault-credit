-- Rename repayment_amount_kes → quoted_repayment_amount_kes and add quoted_at.
--
-- The old name implied "amount the borrower will repay" — a contract. The
-- real amount owed at repayment time is driven by the vault's borrow_index
-- (interest accrues continuously) and current FX. The value persisted at
-- origination is just a quote shown to the user on the USSD confirmation
-- screen; downstream code (SMS reminders, repayment screen) MUST recompute
-- via the loan service's RepaymentQuote helper instead of reading this
-- column. quoted_at captures when the quote was struck for drift analysis.

ALTER TABLE loans RENAME COLUMN repayment_amount_kes TO quoted_repayment_amount_kes;
ALTER TABLE loans ADD COLUMN IF NOT EXISTS quoted_at TIMESTAMP;
