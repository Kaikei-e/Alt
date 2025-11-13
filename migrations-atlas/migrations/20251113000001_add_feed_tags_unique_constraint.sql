-- Migration: add unique constraint on feed_tags (feed_id, tag_name)
-- Created: 2025-11-13
-- Atlas Version: v0.35
-- Purpose: Enable ON CONFLICT (feed_id, tag_name) DO NOTHING in tag insertion

-- Add unique constraint to prevent duplicate tags per feed
CREATE UNIQUE INDEX IF NOT EXISTS feed_tags_feed_id_tag_name_unique
    ON feed_tags (feed_id, tag_name);

-- Note: Using CREATE UNIQUE INDEX instead of ALTER TABLE ADD CONSTRAINT
-- because it's more efficient and allows IF NOT EXISTS

