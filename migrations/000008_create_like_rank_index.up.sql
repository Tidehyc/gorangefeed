-- 000008_create_like_rank_index.up.sql
-- Add compound index on video_stats (like_count DESC, video_id DESC) for high-performance like_rank queries

CREATE INDEX idx_like_rank ON video_stats (like_count DESC, video_id DESC);
