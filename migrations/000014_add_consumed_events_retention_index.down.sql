-- 000014_add_consumed_events_retention_index.down.sql
-- Drop the retention index from consumed_events.

ALTER TABLE consumed_events
    DROP KEY idx_consumed_at;
