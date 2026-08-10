-- Expiry for the /r/{code} interactive redirect.
--
-- The code is an unauthenticated bearer token resolving to a live SEP-24 KYC
-- session. Until now it was gated only on disbursement status, so a loan stuck
-- in processing left the code valid indefinitely. Existing rows get no expiry
-- and keep the old status-only behaviour.

ALTER TABLE loans ADD COLUMN IF NOT EXISTS ramp_short_code_expires_at TIMESTAMPTZ;
