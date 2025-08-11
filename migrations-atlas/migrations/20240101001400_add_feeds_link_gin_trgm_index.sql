-- Migration: add feeds link gin trgm index
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000014_add_feeds_link_gin_trgm_index.up.sql

-- Enable pg_trgm extension for trigram similarity operations
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Add GIN trigram index on feeds.link for fast text similarity searches
CREATE INDEX IF NOT EXISTS idx_feeds_link_gin_trgm ON feeds USING gin (link gin_trgm_ops);
