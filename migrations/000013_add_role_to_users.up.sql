-- 000013_add_role_to_users.up.sql
-- Add a permission scope to users so administrative moderation endpoints can be gated by RBAC.
-- The column is added last and combined with its index in a single statement: MySQL 8 can apply
-- an append-only ADD COLUMN as an INSTANT change instead of rebuilding the table.

ALTER TABLE users
    ADD COLUMN role VARCHAR(16) NOT NULL DEFAULT 'user',
    ADD KEY idx_role (role);
