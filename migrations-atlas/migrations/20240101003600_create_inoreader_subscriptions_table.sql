-- Migration: create inoreader subscriptions table
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000037_create_inoreader_subscriptions_table.up.sql

-- Create inoreader_subscriptions table for storing Inoreader feed subscriptions
CREATE TABLE IF NOT EXISTS inoreader_subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inoreader_id TEXT UNIQUE NOT NULL,
    feed_url TEXT NOT NULL,
    title TEXT,
    category TEXT,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_inoreader_subscriptions_inoreader_id ON inoreader_subscriptions(inoreader_id);
CREATE INDEX IF NOT EXISTS idx_inoreader_subscriptions_feed_url ON inoreader_subscriptions(feed_url);
CREATE INDEX IF NOT EXISTS idx_inoreader_subscriptions_synced_at ON inoreader_subscriptions(synced_at DESC);

-- Add comments for documentation
COMMENT ON TABLE inoreader_subscriptions IS 'Stores RSS feed subscriptions synchronized from Inoreader API';
COMMENT ON COLUMN inoreader_subscriptions.id IS 'Internal UUID primary key';
COMMENT ON COLUMN inoreader_subscriptions.inoreader_id IS 'Unique identifier from Inoreader API (e.g., feed/http://example.com/rss)';
COMMENT ON COLUMN inoreader_subscriptions.feed_url IS 'XML RSS feed URL';
COMMENT ON COLUMN inoreader_subscriptions.title IS 'Feed title from Inoreader';
COMMENT ON COLUMN inoreader_subscriptions.category IS 'Feed category/folder from Inoreader';
COMMENT ON COLUMN inoreader_subscriptions.synced_at IS 'Last synchronization timestamp';
COMMENT ON COLUMN inoreader_subscriptions.created_at IS 'Record creation timestamp';
