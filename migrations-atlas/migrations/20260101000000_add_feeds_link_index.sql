-- Migration: add index on feeds.link for faster URL lookups
-- Created: 2026-01-01
-- Purpose: Optimize MarkAsRead API by enabling O(1) feed lookup via indexed WHERE clause
-- Atlas Version: v0.35+

-- Add index on link column for faster URL lookups
-- Note: Not using CONCURRENTLY as Atlas runs migrations in transactions
CREATE INDEX IF NOT EXISTS idx_feeds_link ON feeds (link);

-- Add comment explaining the index purpose
COMMENT ON INDEX idx_feeds_link IS 'Index for fast URL lookups in MarkAsRead API (SELECT id FROM feeds WHERE link = ?)';
