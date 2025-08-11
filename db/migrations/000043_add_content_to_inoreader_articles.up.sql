-- Add content fields to inoreader_articles table for storing article full content
-- Phase 1: Database Schema Extension for Article Content Storage

ALTER TABLE inoreader_articles 
ADD COLUMN content TEXT,
ADD COLUMN content_length INTEGER DEFAULT 0,
ADD COLUMN content_type VARCHAR(50) DEFAULT 'html';

-- Update comments for new columns
COMMENT ON COLUMN inoreader_articles.content IS 'Full article content from Inoreader summary.content field';
COMMENT ON COLUMN inoreader_articles.content_length IS 'Length of content in characters for optimization';
COMMENT ON COLUMN inoreader_articles.content_type IS 'Content type (html, html_rtl, text)';

-- Create index for content-based queries (partial index for performance)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inoreader_articles_has_content
ON inoreader_articles(content_length) 
WHERE content_length > 0;

-- Create composite index for processed status and content availability
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inoreader_articles_processed_content
ON inoreader_articles(processed, content_length) 
WHERE content_length > 0;

-- Create index for content type filtering
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_inoreader_articles_content_type
ON inoreader_articles(content_type)
WHERE content_type IS NOT NULL AND content_type != 'html';