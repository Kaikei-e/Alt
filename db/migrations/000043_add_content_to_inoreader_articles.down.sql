-- Rollback content fields from inoreader_articles table
-- Phase 1: Database Schema Extension Rollback

-- Drop indexes first
DROP INDEX CONCURRENTLY IF EXISTS idx_inoreader_articles_content_type;
DROP INDEX CONCURRENTLY IF EXISTS idx_inoreader_articles_processed_content;
DROP INDEX CONCURRENTLY IF EXISTS idx_inoreader_articles_has_content;

-- Drop content columns
ALTER TABLE inoreader_articles DROP COLUMN IF EXISTS content_type;
ALTER TABLE inoreader_articles DROP COLUMN IF EXISTS content_length;
ALTER TABLE inoreader_articles DROP COLUMN IF EXISTS content;