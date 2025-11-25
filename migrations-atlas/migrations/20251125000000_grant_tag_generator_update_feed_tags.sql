-- Migration: grant tag_generator UPDATE permission on feed_tags
-- Created: 2025-11-25
-- Purpose: Allow tag_generator to update confidence values in feed_tags table
--          This is required for Phase 1 improvement: tag confidence calculation

-- Grant UPDATE permission on feed_tags table to tag_generator user
GRANT UPDATE ON feed_tags TO tag_generator;

