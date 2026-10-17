-- Optimize Knowledge Sovereign indexes (P1-5):
-- 1. Drop idx_kh_items_user_score, which lacks projection_version and is
--    superseded by idx_khi_user_version_score / idx_kh_items_user_version_visible_score.
-- 2. Add partial composite index on (user_id, projection_version, primary_ref_id)
--    for AreArticlesVisibleInLens stream visibility checks.
-- 3. Add partial composite index on (user_id, projection_version, published_at DESC)
--    for recency-bounded candidate selection in GetKnowledgeHomeItems and
--    DeriveTrailClusterCandidates.
-- 4. Add composite index on knowledge_trail_footprints (user_id, item_key)
--    for item-level wear aggregation and cluster candidate anti-joins.

DROP INDEX IF EXISTS idx_kh_items_user_score;

CREATE INDEX IF NOT EXISTS idx_khi_user_version_primary_ref
  ON knowledge_home_items (user_id, projection_version, primary_ref_id)
  WHERE dismissed_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_khi_user_dismissed_published
  ON knowledge_home_items (user_id, projection_version, published_at DESC)
  WHERE dismissed_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_trail_footprints_user_item_key
  ON knowledge_trail_footprints (user_id, item_key);
