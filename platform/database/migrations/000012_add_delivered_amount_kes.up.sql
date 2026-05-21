-- Add delivered_amount_kes — what the borrower actually received in fiat,
-- net of provider fees. Distinct from:
--   - requested_local_amount: what the borrower asked for at quote time
--   - ramp_fiat_amount:       what YC/MG committed to before fees
--
-- Populated when the off-ramp confirms completion (DisbursementComplete
-- webhook for YC, poller transition for MG cash-pickup). NULL while a loan
-- is in flight, and stays NULL on permanent failure.

ALTER TABLE loans ADD COLUMN IF NOT EXISTS delivered_amount_kes BIGINT;
