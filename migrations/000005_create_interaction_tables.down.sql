-- 000005_create_interaction_tables.down.sql
-- Rollback likes, comments, and comment_likes tables

DROP TABLE IF EXISTS comment_likes;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS likes;
