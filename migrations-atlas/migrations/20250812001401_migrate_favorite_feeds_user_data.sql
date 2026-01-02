-- Migrate existing favorite_feeds data to single user
-- Phase 2: Data migration and schema finalization
-- This migration assumes there is only one user in the system

-- Update all existing favorite_feeds to belong to the single user
-- DEPRECATED: Default user ID no longer used (now using Kratos identity)
-- User ID: 572d583d-dcc4-4ffc-b6b1-9cd0181401ee (from logs)
-- UPDATE favorite_feeds
-- SET user_id = '572d583d-dcc4-4ffc-b6b1-9cd0181401ee'::UUID
-- WHERE user_id IS NULL;

-- Make user_id NOT NULL now that all rows have a value
ALTER TABLE favorite_feeds
ALTER COLUMN user_id SET NOT NULL;

-- Drop the old single-column primary key
ALTER TABLE favorite_feeds
DROP CONSTRAINT IF EXISTS favorite_feeds_pkey;

-- Create composite primary key (user_id, feed_id)
-- This allows multiple users to favorite the same feed
ALTER TABLE favorite_feeds
ADD PRIMARY KEY (user_id, feed_id);

-- Create index for efficient user-based queries ordered by creation time
CREATE INDEX IF NOT EXISTS idx_favorite_feeds_user_created
ON favorite_feeds (user_id, created_at DESC);

-- Add comment for documentation
COMMENT ON COLUMN favorite_feeds.user_id IS 'User who favorited the feed - supports multi-tenant architecture';