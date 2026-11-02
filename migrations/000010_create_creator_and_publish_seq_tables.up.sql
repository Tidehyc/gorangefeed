-- 000010_create_creator_and_publish_seq_tables.up.sql
-- Create drafts and scheduled_tasks tables, and add publish_seq to videos table

CREATE TABLE IF NOT EXISTS drafts (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    author_id BIGINT NOT NULL,
    title VARCHAR(128) NOT NULL DEFAULT '',
    description VARCHAR(1000) NOT NULL DEFAULT '',
    play_url VARCHAR(512) NOT NULL DEFAULT '',
    cover_url VARCHAR(512) NOT NULL DEFAULT '',
    duration_ms INT NOT NULL DEFAULT 0,
    status TINYINT NOT NULL DEFAULT 1 COMMENT '1: draft, 2: scheduled, 3: published, 4: deleted',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY idx_author_updated_id (author_id, updated_at DESC, id DESC),
    KEY idx_author_status (author_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS scheduled_tasks (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    task_id CHAR(36) NOT NULL UNIQUE,
    author_id BIGINT NOT NULL,
    draft_id BIGINT NULL DEFAULT NULL,
    title VARCHAR(128) NOT NULL DEFAULT '',
    description VARCHAR(1000) NOT NULL DEFAULT '',
    play_url VARCHAR(512) NOT NULL DEFAULT '',
    cover_url VARCHAR(512) NOT NULL DEFAULT '',
    duration_ms INT NOT NULL DEFAULT 0,
    scheduled_at DATETIME(3) NOT NULL,
    status TINYINT NOT NULL DEFAULT 1 COMMENT '1: scheduled, 2: executing, 3: published, 4: canceled, 5: failed',
    video_id BIGINT NULL DEFAULT NULL,
    error_msg TEXT NULL DEFAULT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY idx_author_scheduled (author_id, scheduled_at DESC, id DESC),
    KEY idx_status_scheduled_at (status, scheduled_at),
    KEY idx_draft_id (draft_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE videos ADD COLUMN publish_seq BIGINT NULL DEFAULT NULL;
UPDATE videos SET publish_seq = id WHERE publish_seq IS NULL;
ALTER TABLE videos ADD UNIQUE KEY uk_publish_seq (publish_seq);
