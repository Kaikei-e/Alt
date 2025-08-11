-- Migration: add feeds desc not mp3 cover index
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000023_add_feeds_desc_not_mp3_cover_index.up.sql

CREATE INDEX IF NOT EXISTS idx_feeds_desc_not_mp3_cover
  ON feeds (created_at DESC, id DESC)
  INCLUDE (link)
  WHERE link NOT LIKE '%.mp3';
