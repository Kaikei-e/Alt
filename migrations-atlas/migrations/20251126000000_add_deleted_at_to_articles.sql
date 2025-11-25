-- Migration: add deleted_at column for soft delete support
-- Created: 2025-11-26
-- Purpose: Enable soft delete for articles to sync deletions with Meilisearch

ALTER TABLE articles ADD COLUMN deleted_at TIMESTAMP;
CREATE INDEX idx_articles_deleted_at ON articles(deleted_at) WHERE deleted_at IS NOT NULL;

