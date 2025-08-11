-- Migration: add feeds created desc not mp3 index
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000018_add_feeds_created_desc_not_mp3_index.up.sql

-- Add partial index for feeds excluding MP3 files, ordered by creation date descending
-- Optimizes queries for non-audio content chronologically
CREATE INDEX idx_feeds_created_desc_not_mp3
  ON feeds (created_at DESC, link)
  WHERE link NOT LIKE '%.mp3';
