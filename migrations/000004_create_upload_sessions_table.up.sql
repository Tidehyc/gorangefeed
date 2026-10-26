CREATE TABLE IF NOT EXISTS upload_sessions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    upload_id VARCHAR(64) NOT NULL,
    user_id BIGINT NOT NULL,
    file_hash CHAR(64) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    file_size BIGINT NOT NULL,
    chunk_size INT NOT NULL,
    total_chunks INT NOT NULL,
    status TINYINT NOT NULL DEFAULT 1 COMMENT '1: uploading, 2: merging, 3: completed, 4: expired',
    play_url VARCHAR(512) DEFAULT NULL,
    expire_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_upload_id (upload_id),
    KEY idx_user_file_hash (user_id, file_hash, status),
    KEY idx_user_id (user_id),
    KEY idx_expire_at (expire_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
