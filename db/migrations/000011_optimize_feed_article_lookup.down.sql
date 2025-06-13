-- Remove optimization indexes
DROP INDEX IF EXISTS idx_articles_url_lookup;
DROP INDEX IF EXISTS idx_feeds_created_at_link;
-- DROP INDEX IF EXISTS idx_articles_url_hash;