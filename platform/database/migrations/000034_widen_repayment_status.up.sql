-- partial_funds_received is 22 chars, over the old varchar(20).
ALTER TABLE loans ALTER COLUMN repayment_status TYPE varchar(30);
