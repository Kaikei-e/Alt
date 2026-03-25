-- Add covering partial index for the correlated subquery that finds the latest
-- article per feed. This pattern appears in FetchFeedsByFeedLinkID,
-- FetchUnreadFeedsListCursor, FetchAllFeedsListCursor, and
-- FetchFavoriteFeedsListCursor.
--
-- Optimizes:
--   (SELECT a.id FROM articles a
--    WHERE a.feed_id = f.id AND a.deleted_at IS NULL
--    ORDER BY a.created_at DESC LIMIT 1)
--
-- INCLUDE (id) enables index-only scan since the subquery only selects a.id.
-- WHERE deleted_at IS NULL excludes soft-deleted rows (partial index).
CREATE INDEX IF NOT EXISTS idx_articles_feed_active_created_desc
  ON articles (feed_id, created_at DESC)
  INCLUDE (id)
  WHERE deleted_at IS NULL;
