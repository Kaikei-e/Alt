-- Optimize indexes: add missing indexes and drop redundant ones
-- Part of the SQL comprehensive review

-- =====================================================
-- ADD NEW INDEXES
-- =====================================================

-- soft-delete filtered article URL lookup (replaces unfiltered idx_articles_url_lookup)
CREATE INDEX IF NOT EXISTS idx_articles_url_not_deleted
  ON articles (url) WHERE deleted_at IS NULL;

-- soft-delete filtered user article listing for cursor pagination
CREATE INDEX IF NOT EXISTS idx_articles_active_user_created
  ON articles (user_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;

-- read_status cursor pagination by read_at (for FetchReadFeedsListCursor)
CREATE INDEX IF NOT EXISTS idx_read_status_user_read_at_desc
  ON read_status (user_id, read_at DESC) WHERE is_read = TRUE;

-- FetchUserFeedIDs index-only scan
CREATE INDEX IF NOT EXISTS idx_read_status_user_feed
  ON read_status (user_id, feed_id);

-- =====================================================
-- DROP REDUNDANT INDEXES
-- =====================================================

-- idx_feeds_created_at is a left prefix of idx_feeds_created_at_link
DROP INDEX IF EXISTS idx_feeds_created_at;

-- Low selectivity boolean index, covered by partial indexes and composites
DROP INDEX IF EXISTS idx_read_status_is_read;

-- Standalone feed_id index is covered by composites (feed_id, user_id) UNIQUE + (feed_id, is_read)
DROP INDEX IF EXISTS idx_read_status_feed_id;

-- user_reading_status indexes redundant with UNIQUE constraint
DROP INDEX IF EXISTS idx_user_reading_status_user_article;
DROP INDEX IF EXISTS idx_user_reading_status_user_id;

-- feed_links composite UNIQUE (id, url) is redundant with PK on id + UNIQUE on url
ALTER TABLE feed_links DROP CONSTRAINT IF EXISTS idx_feed_links_id_url;
