-- Move the loan FX/rate columns off floating point.
--
-- disbursement_rate and entry_rate_buffered are FX rates (e.g. 128.23 KES/USD),
-- not percentages, so they keep their names and become integers scaled by 10^8
-- — the precision NUMERIC(20,8) carried, without the float64 the Go model read
-- them into. The scale is not in the column name: see migration 000019, which
-- removed a _bps suffix from disbursement_rate because a wrong unit in a name
-- is worse than no unit. models.RateE8 / models.RateFromE8 own the conversion.
--
-- entry_buffer_pct is a genuine percentage, held as a fraction (0.0100 = 1 %).
-- Basis points are its natural integer unit, so it becomes entry_buffer_bps
-- (0.0100 -> 100). Here the suffix is a unit, not a scale, and is accurate.

ALTER TABLE loans
    ALTER COLUMN disbursement_rate TYPE BIGINT
    USING round(disbursement_rate * 100000000)::bigint;

ALTER TABLE loans
    ALTER COLUMN entry_rate_buffered TYPE BIGINT
    USING round(entry_rate_buffered * 100000000)::bigint;

ALTER TABLE loans RENAME COLUMN entry_buffer_pct TO entry_buffer_bps;
ALTER TABLE loans
    ALTER COLUMN entry_buffer_bps TYPE INTEGER
    USING round(entry_buffer_bps * 10000)::integer;
