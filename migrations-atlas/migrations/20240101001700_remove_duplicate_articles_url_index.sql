-- Migration: remove duplicate articles url index
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000017_remove_duplicate_articles_url_index.up.sql

-- Remove duplicate index on articles.url
-- The unique index idx_articles_url already provides lookup performance
DROP INDEX IF EXISTS idx_articles_url_lookup;
