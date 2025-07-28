-- Add GIN trigram index for article_summaries table
-- Index on article_title for fast text similarity searches
CREATE INDEX IF NOT EXISTS idx_article_summaries_title_gin_trgm ON article_summaries USING gin (article_title gin_trgm_ops);