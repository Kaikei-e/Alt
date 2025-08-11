-- Migration: add article summaries gin trgm indexes
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000016_add_article_summaries_gin_trgm_indexes.up.sql

-- Add GIN trigram index for article_summaries table
-- Index on article_title for fast text similarity searches
CREATE INDEX IF NOT EXISTS idx_article_summaries_title_gin_trgm ON article_summaries USING gin (article_title gin_trgm_ops);
