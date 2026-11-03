-- 000011_create_favorites_and_history_tables.up.sql
-- Create favorites, favorite_items, watch_history tables and add favorite_count to video_stats

CREATE TABLE IF NOT EXISTS favorites (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    name VARCHAR(64) NOT NULL DEFAULT '',
    description VARCHAR(255) NOT NULL DEFAULT '',
    is_public TINYINT NOT NULL DEFAULT 1 COMMENT '1: public, 0: private',
    is_default TINYINT NULL DEFAULT NULL COMMENT '1: default favorite folder, NULL: custom folder',
    item_count BIGINT NOT NULL DEFAULT 0 COMMENT 'total videos favorited in this folder',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY idx_user_id (user_id),
    UNIQUE KEY uk_user_default (user_id, is_default)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS favorite_items (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    favorite_id BIGINT NOT NULL,
    video_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_favorite_video (favorite_id, video_id),
    KEY idx_user_video (user_id, video_id),
    KEY idx_favorite_created (favorite_id, created_at DESC, id DESC),
    KEY idx_video_id (video_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS watch_history (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    video_id BIGINT NOT NULL,
    last_position_ms INT NOT NULL DEFAULT 0 COMMENT 'playback position in milliseconds',
    duration_ms INT NOT NULL DEFAULT 0 COMMENT 'video duration in milliseconds',
    is_finished TINYINT NOT NULL DEFAULT 0 COMMENT '1: playback finished, 0: partial',
    watched_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'timestamp of last playback',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_user_video (user_id, video_id),
    KEY idx_user_watched (user_id, watched_at DESC, id DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE video_stats ADD COLUMN favorite_count BIGINT NOT NULL DEFAULT 0;
