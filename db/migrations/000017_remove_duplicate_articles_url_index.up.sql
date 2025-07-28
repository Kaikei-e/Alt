-- Remove duplicate index on articles.url
-- The unique index idx_articles_url already provides lookup performance
DROP INDEX IF EXISTS idx_articles_url_lookup;