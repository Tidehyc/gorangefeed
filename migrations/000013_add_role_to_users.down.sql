-- 000013_add_role_to_users.down.sql
-- Drop the RBAC role column and its index from users.

ALTER TABLE users
    DROP KEY idx_role,
    DROP COLUMN role;
