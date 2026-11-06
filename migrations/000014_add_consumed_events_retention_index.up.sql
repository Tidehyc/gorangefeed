-- 000014_add_consumed_events_retention_index.up.sql
-- Idempotency markers only need to outlive the broker's redelivery window, but nothing ever
-- removed them, so consumed_events grew for the lifetime of the service. The worker now purges
-- rows older than a retention window; this index makes that DELETE a range scan instead of a
-- full table scan.

ALTER TABLE consumed_events
    ADD KEY idx_consumed_at (consumed_at);
