-- 000003_create_videos_table.down.sql
-- Rollback videos and video_stats tables

DROP TABLE IF EXISTS video_stats;
DROP TABLE IF EXISTS videos;
