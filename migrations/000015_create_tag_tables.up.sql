-- 000015_create_tag_tables.up.sql
-- Create the tag dictionary and the video-tag relation for M11-01 (video classification /
-- short-term interest modelling). The dictionary exists so that spelling variants collapse into
-- one dimension: a cosine similarity over tag vectors is meaningless if "#美食" and "#美食 " are
-- counted as two different features.

CREATE TABLE IF NOT EXISTS tags (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(32) NOT NULL COMMENT 'Normalized key: trimmed, lowercased, no leading #',
    display_name VARCHAR(32) NOT NULL COMMENT 'Original casing kept for rendering',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS video_tags (
    video_id BIGINT NOT NULL,
    tag_id BIGINT NOT NULL,
    weight DECIMAL(4,3) NOT NULL DEFAULT 1.000 COMMENT 'Per-video tag strength, used by TagSimilarity',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (video_id, tag_id),
    KEY idx_tag_video (tag_id, video_id DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
