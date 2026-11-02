CREATE TABLE IF NOT EXISTS `notifications` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `recipient_id` BIGINT UNSIGNED NOT NULL COMMENT '接收通知用户ID',
    `actor_id` BIGINT UNSIGNED NOT NULL COMMENT '触发行为用户ID',
    `type` VARCHAR(32) NOT NULL COMMENT '通知类型: like, comment, follow, mention, system',
    `target_type` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '目标主体类型: video, comment, user',
    `target_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '目标主体ID',
    `title` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '通知简要标题',
    `content` VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '通知富文本内容或评论摘要',
    `event_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '全局业务事件UUID',
    `is_read` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '0:未读, 1:已读',
    `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间(毫秒)',
    `read_at` DATETIME(3) NULL DEFAULT NULL COMMENT '标记已读时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_recipient_event` (`recipient_id`, `event_id`),
    KEY `idx_recipient_created_id` (`recipient_id`, `created_at` DESC, `id` DESC),
    KEY `idx_recipient_unread` (`recipient_id`, `is_read`),
    KEY `idx_actor` (`actor_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='系统通知明细表';
