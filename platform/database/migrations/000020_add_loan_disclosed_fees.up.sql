-- Disclosed fees: the admin charges communicated to the borrower at borrow.
--
-- Kept separate from service_fee_* and partner_fee_*, which the ramp reports
-- after settlement (disbursement_status_adapter writes them from the YellowCard
-- webhook, the poller from MoneyGram). Those are what the provider actually
-- charged and are unknowable at borrow time; these are ours and deterministic.
--
-- Sharing columns between the two would let the settlement write destroy the
-- disclosed figure, leaving no way to answer what the borrower was told —
-- which is the reason for disclosing them in the first place. So these are
-- written once at creation and never rewritten.
--
-- Interest is deliberately absent: it accrues against the vault's borrow_index
-- and borrowers are told to check their balance, so there is no fixed interest
-- figure to disclose. See migration 000019.
--
-- Both denominations are stored, matching the service_fee_*/partner_fee_*
-- pairing: local is what the borrower sees, USD is what reconciles against
-- treasury. Amounts are minor units (cents/stroops), consistent with every
-- other amount column on this table.
ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS telco_fee_usd BIGINT,
    ADD COLUMN IF NOT EXISTS telco_fee_local BIGINT,
    ADD COLUMN IF NOT EXISTS tax_usd BIGINT,
    ADD COLUMN IF NOT EXISTS tax_local BIGINT;
