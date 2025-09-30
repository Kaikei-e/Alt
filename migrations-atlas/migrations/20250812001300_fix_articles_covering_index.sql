-- Fix idx_articles_cover_desc to avoid B-tree index size limit
-- Atlas Migration: Corresponds to db/migrations/000042_fix_articles_covering_index.up.sql
-- PostgreSQL B-tree indexes have a maximum size of ~2704 bytes
-- The previous index included the full 'content' field which can exceed this limit

-- Drop the problematic covering index that includes the full content field
DROP INDEX IF EXISTS idx_articles_cover_desc;

-- Recreate without 'content' in the INCLUDE clause
-- This avoids PostgreSQL B-tree index size limit (~2704 bytes)
-- The content field will be retrieved from the table when needed (heap access)
CREATE INDEX IF NOT EXISTS idx_articles_cover_desc
  ON articles (created_at DESC, id DESC)
  INCLUDE (title, url);