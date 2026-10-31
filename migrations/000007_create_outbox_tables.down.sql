-- 000007_create_outbox_tables.down.sql
-- Rollback outbox and consumed_events tables

DROP TABLE IF EXISTS consumed_events;
DROP TABLE IF EXISTS outbox;
