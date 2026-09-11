-- Paybill repayment is walk-up-and-pay: nothing initiates it the way an STK
-- push or a MoneyGram deposit does, so a loan's progress toward its payoff
-- accumulates from mpesa_transactions.applied_stroops (core DB) rather than
-- being written by a single settling call. repayment_received_stroops is a
-- cached display total, recomputed from that sum on every sweep tick — it is
-- never itself the source of truth, only a copy of it.

ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS repayment_received_stroops bigint;
