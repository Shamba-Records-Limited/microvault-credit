-- Recreate the column. No backfill from requested_local_amount — the
-- original column held requested-amount semantics that are already
-- preserved there, so down is a no-data recreate.
ALTER TABLE loans ADD COLUMN IF NOT EXISTS disbursement_amount_kes BIGINT;
