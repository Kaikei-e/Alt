-- Drop unused indexes (idx_scan = 0) to reclaim disk space and reduce write amplification
DROP INDEX IF EXISTS idx_feeds_website_url_desc_not_mp3_cover;
DROP INDEX IF EXISTS idx_feeds_website_url_gin_trgm;
DROP INDEX IF EXISTS idx_feeds_website_url_created_desc_not_mp3;
DROP INDEX IF EXISTS idx_articles_title_gin_trgm;
DROP INDEX IF EXISTS idx_feeds_website_url;
DROP INDEX IF EXISTS idx_articles_url_gin_trgm;
DROP INDEX IF EXISTS idx_article_tags_feed_tag_id;
