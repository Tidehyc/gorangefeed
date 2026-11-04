-- 000012_create_moderation_tables.up.sql
-- Create review_tasks and moderation_logs tables for M9 Content Moderation domain

CREATE TABLE IF NOT EXISTS review_tasks (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    video_id BIGINT NOT NULL,
    policy TINYINT NOT NULL DEFAULT 1 COMMENT '1: post_review, 2: pre_review',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '1: pending_machine, 2: pending_human, 3: approved, 4: rejected, 5: takedown',
    risk_score INT NOT NULL DEFAULT 0,
    matched_words JSON NULL COMMENT 'Matched sensitive words',
    reviewer_id BIGINT NOT NULL DEFAULT 0,
    review_note VARCHAR(500) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_video_id (video_id),
    KEY idx_status_id (status, id DESC),
    KEY idx_reviewer_status (reviewer_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS moderation_logs (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    target_type VARCHAR(32) NOT NULL COMMENT 'video / user',
    target_id BIGINT NOT NULL,
    action VARCHAR(32) NOT NULL COMMENT 'machine_approve / machine_reject / human_approve / human_reject / takedown / ban / unban',
    operator_id BIGINT NOT NULL DEFAULT 0 COMMENT '0 for system machine, >0 for human admin',
    reason VARCHAR(500) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY idx_target (target_type, target_id, id DESC),
    KEY idx_operator (operator_id, id DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
