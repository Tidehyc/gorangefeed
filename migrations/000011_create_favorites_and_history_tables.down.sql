-- 000011_create_favorites_and_history_tables.down.sql
-- Rollback favorites, favorite_items, watch_history tables and favorite_count column

ALTER TABLE video_stats DROP COLUMN favorite_count;
DROP TABLE IF EXISTS watch_history;
DROP TABLE IF EXISTS favorite_items;
DROP TABLE IF EXISTS favorites;
