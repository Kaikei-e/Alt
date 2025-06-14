-- Remove GIN trigram indexes for articles table
DROP INDEX IF EXISTS idx_articles_title_gin_trgm;
DROP INDEX IF EXISTS idx_articles_url_gin_trgm;