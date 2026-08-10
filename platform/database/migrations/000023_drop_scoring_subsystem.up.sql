-- Remove the credit-scoring / alt-data subsystem. These tables backed the
-- document-ingest, transaction-extraction, cashflow, farm-record, credit-factor,
-- credit-score, scoring-factor, risk-tier and config-audit models — none of
-- which drive the current lending flow, which runs on loans, loan_products,
-- repayments, transactions and the two config tables kept below.
-- CASCADE handles the inter-table foreign keys regardless of drop order.
DROP TABLE IF EXISTS credit_factors CASCADE;
DROP TABLE IF EXISTS transactions_extracted CASCADE;
DROP TABLE IF EXISTS farm_records CASCADE;
DROP TABLE IF EXISTS cashflow_analyses CASCADE;
DROP TABLE IF EXISTS user_documents CASCADE;
DROP TABLE IF EXISTS credit_scores CASCADE;
DROP TABLE IF EXISTS credit_scoring_factors CASCADE;
DROP TABLE IF EXISTS risk_tier_configs CASCADE;
DROP TABLE IF EXISTS credit_config_audit_logs CASCADE;
