-- Migration: create inoreader articles table
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000038_create_inoreader_articles_table.up.sql

-- Create inoreader_articles table for storing article metadata from Inoreader
CREATE TABLE IF NOT EXISTS inoreader_articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inoreader_id TEXT UNIQUE NOT NULL,
    subscription_id UUID REFERENCES inoreader_subscriptions(id) ON DELETE CASCADE,
    article_url TEXT NOT NULL,
    title TEXT,
    author TEXT,
    published_at TIMESTAMP WITH TIME ZONE,
    fetched_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processed BOOLEAN DEFAULT FALSE
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_inoreader_id ON inoreader_articles(inoreader_id);
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_subscription_id ON inoreader_articles(subscription_id);
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_article_url ON inoreader_articles(article_url);
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_published_at ON inoreader_articles(published_at DESC);
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_fetched_at ON inoreader_articles(fetched_at DESC);
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_processed ON inoreader_articles(processed) WHERE processed = FALSE;

-- Add comments for documentation
COMMENT ON TABLE inoreader_articles IS 'Stores article metadata fetched from Inoreader stream contents API';
COMMENT ON COLUMN inoreader_articles.id IS 'Internal UUID primary key';
COMMENT ON COLUMN inoreader_articles.inoreader_id IS 'Unique article identifier from Inoreader API';
COMMENT ON COLUMN inoreader_articles.subscription_id IS 'Reference to inoreader_subscriptions table';
COMMENT ON COLUMN inoreader_articles.article_url IS 'URL to the original article';
COMMENT ON COLUMN inoreader_articles.title IS 'Article title from Inoreader';
COMMENT ON COLUMN inoreader_articles.author IS 'Article author';
COMMENT ON COLUMN inoreader_articles.published_at IS 'Original publication timestamp';
COMMENT ON COLUMN inoreader_articles.fetched_at IS 'When this record was fetched from Inoreader';
COMMENT ON COLUMN inoreader_articles.processed IS 'Whether this article has been processed by other services';
