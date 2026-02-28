-- Migration: add published_at column to articles table
-- Purpose: Enable storing original publication timestamp from RSS feeds
-- Required by: CreateArticleInternal driver (alt-backend/app/driver/alt_db/create_article_driver.go)

ALTER TABLE articles ADD COLUMN published_at TIMESTAMPTZ;

COMMENT ON COLUMN articles.published_at IS 'Original publication timestamp from RSS feed source';
