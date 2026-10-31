-- 000007_create_outbox_tables.up.sql
-- Create outbox and consumed_events tables for reliable event publishing and idempotency

CREATE TABLE IF NOT EXISTS outbox (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    event_id CHAR(36) NOT NULL UNIQUE,
    event_type VARCHAR(64) NOT NULL,
    payload JSON NOT NULL,
    status ENUM('pending', 'sending', 'sent', 'dead') NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    next_retry_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    lease_token CHAR(36) NULL DEFAULT NULL,
    lease_until DATETIME(3) NULL DEFAULT NULL,
    last_error TEXT NULL DEFAULT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    sent_at DATETIME(3) NULL DEFAULT NULL,
    KEY idx_dispatch (status, next_retry_at, id),
    KEY idx_lease (status, lease_until),
    KEY idx_lease_token (lease_token)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS consumed_events (
    event_id CHAR(36) PRIMARY KEY,
    consumed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
