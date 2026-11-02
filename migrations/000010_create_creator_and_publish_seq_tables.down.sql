-- 000010_create_creator_and_publish_seq_tables.down.sql
-- Rollback creator and publish_seq tables and columns

DROP TABLE IF EXISTS scheduled_tasks;
DROP TABLE IF EXISTS drafts;
ALTER TABLE videos DROP INDEX uk_publish_seq;
ALTER TABLE videos DROP COLUMN publish_seq;
