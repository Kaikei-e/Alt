-- Migration: Add user_id to articles table for multi-tenant support
-- Created: 2025-10-06
-- Purpose: Enable user-based filtering for article search functionality

-- Add user_id column (nullable initially for data migration)
ALTER TABLE articles ADD COLUMN user_id UUID;

-- Create indexes for efficient filtering
CREATE INDEX idx_articles_user_id ON articles(user_id);
CREATE INDEX idx_articles_user_created ON articles(user_id, created_at DESC);

-- Migrate existing data to single user
-- DEPRECATED: Default user ID no longer used (now using Kratos identity)
-- User ID: 572d583d-dcc4-4ffc-b6b1-9cd0181401ee (from system logs)
-- UPDATE articles
-- SET user_id = '572d583d-dcc4-4ffc-b6b1-9cd0181401ee'::UUID
-- WHERE user_id IS NULL;

-- Make user_id NOT NULL now that all rows have a value
ALTER TABLE articles ALTER COLUMN user_id SET NOT NULL;

-- Add foreign key constraint (optional - only if users table exists)
-- ALTER TABLE articles ADD CONSTRAINT fk_articles_user_id
-- FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Add documentation comment
COMMENT ON COLUMN articles.user_id IS 'Owner of the article - supports multi-tenant architecture for search isolation';
