-- Migration: grant tag_generator permissions for backfill script
-- Created: 2025-11-13
-- Atlas Version: v0.35
-- Purpose: Allow tag_generator to read feeds table and update articles.feed_id

-- Grant SELECT on feeds table (needed for backfill script)
GRANT SELECT ON feeds TO tag_generator;

-- Grant UPDATE on articles.feed_id (needed for backfill script)
GRANT UPDATE (feed_id) ON articles TO tag_generator;

