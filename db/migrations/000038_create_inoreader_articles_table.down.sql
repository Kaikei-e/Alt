-- Drop inoreader_articles table and related indexes
DROP INDEX IF EXISTS idx_inoreader_articles_processed;
DROP INDEX IF EXISTS idx_inoreader_articles_fetched_at;
DROP INDEX IF EXISTS idx_inoreader_articles_published_at;
DROP INDEX IF EXISTS idx_inoreader_articles_article_url;
DROP INDEX IF EXISTS idx_inoreader_articles_subscription_id;
DROP INDEX IF EXISTS idx_inoreader_articles_inoreader_id;
DROP TABLE IF EXISTS inoreader_articles;