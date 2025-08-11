-- Migration: add articles gin trgm indexes
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000015_add_articles_gin_trgm_indexes.up.sql

-- Add GIN trigram indexes for articles table
-- Index on title for fast text similarity searches
CREATE INDEX IF NOT EXISTS idx_articles_title_gin_trgm ON articles USING gin (title gin_trgm_ops);

-- Index on url for fast text similarity searches
CREATE INDEX IF NOT EXISTS idx_articles_url_gin_trgm ON articles USING gin (url gin_trgm_ops);
