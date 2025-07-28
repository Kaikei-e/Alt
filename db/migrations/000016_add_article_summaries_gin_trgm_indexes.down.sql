-- Remove GIN trigram index for article_summaries table
DROP INDEX IF EXISTS idx_article_summaries_title_gin_trgm;