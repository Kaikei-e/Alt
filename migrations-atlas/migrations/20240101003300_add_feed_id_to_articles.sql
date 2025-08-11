-- Migration: add feed id to articles
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000033_add_feed_id_to_articles.up.sql

-- Add feed_id to articles table to link articles to their source feed
ALTER TABLE articles
    ADD COLUMN feed_id UUID;

-- Add a foreign key constraint to ensure data integrity
-- If a feed is deleted, all its articles will be deleted as well.
ALTER TABLE articles
    ADD CONSTRAINT fk_articles_feed_id
        FOREIGN KEY (feed_id)
        REFERENCES feeds(id)
        ON DELETE CASCADE;

-- Add an index on feed_id for faster querying of articles by feed
CREATE INDEX IF NOT EXISTS idx_articles_feed_id ON articles (feed_id);
