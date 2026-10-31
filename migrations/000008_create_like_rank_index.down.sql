-- 000008_create_like_rank_index.down.sql
-- Drop compound index on video_stats

DROP INDEX idx_like_rank ON video_stats;
