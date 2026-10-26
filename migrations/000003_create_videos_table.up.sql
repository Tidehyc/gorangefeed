-- 000003_create_videos_table.up.sql
-- Create videos and video_stats tables for M2 Video domain

CREATE TABLE IF NOT EXISTS videos (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    author_id BIGINT NOT NULL,
    title VARCHAR(128) NOT NULL,
    description VARCHAR(1000) NOT NULL DEFAULT '',
    cover_url VARCHAR(512) NOT NULL DEFAULT '',
    play_url VARCHAR(512) NOT NULL,
    duration_ms INT NOT NULL DEFAULT 0,
    status TINYINT NOT NULL DEFAULT 1 COMMENT '1: published, 2: hidden',
    idempotency_key VARCHAR(128) NULL DEFAULT NULL,
    published_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_author_idempotency (author_id, idempotency_key),
    KEY idx_status_id (status, id DESC),
    KEY idx_author_status_id (author_id, status, id DESC),
    KEY idx_published_at (published_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS video_stats (
    video_id BIGINT PRIMARY KEY,
    like_count BIGINT NOT NULL DEFAULT 0,
    comment_count BIGINT NOT NULL DEFAULT 0,
    view_count BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
