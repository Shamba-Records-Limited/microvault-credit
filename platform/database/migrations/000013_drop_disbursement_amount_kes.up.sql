-- Drop disbursement_amount_kes. It mirrored requested_local_amount (both
-- captured the user's quote-time fiat ask), and the actually-received fiat
-- now lives in delivered_amount_kes (000012).
--
-- Backfill any rows where requested_local_amount is missing but
-- disbursement_amount_kes was set, so historical loans retain the ask.

UPDATE loans
   SET requested_local_amount = disbursement_amount_kes
 WHERE requested_local_amount IS NULL
   AND disbursement_amount_kes IS NOT NULL;

ALTER TABLE loans DROP COLUMN IF EXISTS disbursement_amount_kes;
