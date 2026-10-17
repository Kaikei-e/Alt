-- Optimize feeds, articles, and article_summaries indexes based on pg_stat_statements and pg_stat_user_indexes telemetry:
-- 1. Restore a composite (created_at DESC, id DESC) B-tree index on feeds for FetchFeedsInWindow
--    and ordered feed window scans (idx_feeds_created_at was dropped in 20260227000000).
-- 2. Add a partial index on articles for untagged/unsummarized keyset scans where feed_id IS NOT NULL
--    and deleted_at IS NULL.
-- 3. Add a composite (article_id, user_id) index on article_summaries for FetchUnsummarizedArticlesCountForUser.
-- 4. Drop redundant/unused indexes on feeds.website_url and article_tags.

CREATE INDEX IF NOT EXISTS idx_feeds_created_at_id_desc
  ON feeds (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_articles_untagged_candidate
  ON articles (created_at DESC, id DESC)
  WHERE feed_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_article_summaries_article_user
  ON article_summaries (article_id, user_id);

DROP INDEX IF EXISTS idx_feeds_website_url;
DROP INDEX IF EXISTS idx_feeds_id_website_url;
DROP INDEX IF EXISTS idx_feeds_website_url_gin_trgm;
DROP INDEX IF EXISTS idx_feeds_website_url_created_desc_not_mp3;
DROP INDEX IF EXISTS idx_feeds_website_url_desc_not_mp3_cover;
DROP INDEX IF EXISTS idx_article_tags_feed_tag_id;
