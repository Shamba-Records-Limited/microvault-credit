-- Remove the borrow-time repayment projection, and rename the columns whose
-- names misstate their own contents.
--
-- interest_amount, total_amount and quoted_repayment_amount_kes were all
-- computed once at creation to populate a USSD confirmation screen:
--
--     interest = principal * apr * duration / 365
--     total    = principal + interest + origination_fee
--     quoted   = total * conversion_rate
--
-- Borrowers are told to check their balance rather than quoted a figure at
-- borrow, because interest accrues against the vault's borrow_index and the
-- rate moves. The code already disowned these values in comments; this removes
-- them so there is one answer to "what do I owe" — GetRepaymentQuote, computed
-- on demand from borrow_index and a live rate.
--
-- The origination fee stays: it is fixed, ours, and disclosed up front.
ALTER TABLE loans
    DROP COLUMN IF EXISTS interest_amount,
    DROP COLUMN IF EXISTS total_amount,
    DROP COLUMN IF EXISTS quoted_repayment_amount_kes,
    DROP COLUMN IF EXISTS quoted_at;

-- momo_* were vestigial from a pre-YellowCard design that would have integrated
-- M-Pesa directly. Nothing ever populated them: the service copied them from
-- update requests and echoed them back, and no caller set them. YellowCard
-- returns no mobile-money receipt at all — not in PaymentDetails, not in the
-- webhook — so momo_transaction_id was unpopulatable by construction. The
-- payout network YC used now lives on the off_ramp transaction's metadata.
--
-- If direct mobile-money rails are ever built, these come back as real columns
-- fed by a real integration.
ALTER TABLE loans
    DROP COLUMN IF EXISTS momo_provider,
    DROP COLUMN IF EXISTS momo_transaction_id,
    DROP COLUMN IF EXISTS momo_status;

-- interest_rate_bps read as a fixed contractual rate. It is neither fixed nor
-- contractual: it is the vault APR sampled at borrow time (product config only
-- when the vault call fails), kept as an audit record of the rate in force.
ALTER TABLE loans RENAME COLUMN interest_rate_bps TO vault_apr_bps;

-- entry_rate_used already has entry_buffer_pct deducted, so comparing it to a
-- live provider quote shows a gap that looks like an error and is not.
ALTER TABLE loans RENAME COLUMN entry_rate_used TO entry_rate_buffered;

-- Currency belongs in ramp_fiat_currency, not in a column name.
ALTER TABLE loans RENAME COLUMN delivered_amount_kes TO delivered_amount_local;

-- disbursement_rate_bps was never basis points. It held the FX rate scaled by
-- 10^4 (128.23 KES/USD stored as 1282300), so a reader trusting the suffix is
-- wrong by four orders of magnitude. Retyping to numeric makes it directly
-- comparable to entry_rate_buffered.
ALTER TABLE loans RENAME COLUMN disbursement_rate_bps TO disbursement_rate;
ALTER TABLE loans
    ALTER COLUMN disbursement_rate TYPE NUMERIC(20,8)
    USING (disbursement_rate::numeric / 10000);
