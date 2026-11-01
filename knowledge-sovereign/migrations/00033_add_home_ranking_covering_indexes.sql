-- Add covering index for late-materialized Index-Only Scan ranking in
-- GetKnowledgeHomeItems and expression index for 30-day recency window in
-- DeriveTrailClusterCandidates.

CREATE INDEX IF NOT EXISTS idx_khi_home_rank_covering
  ON knowledge_home_items (user_id, projection_version)
  INCLUDE (score, published_at, generated_at, item_key)
  WHERE dismissed_at IS NULL AND COALESCE(title, '') <> '';

CREATE INDEX IF NOT EXISTS idx_khi_user_ver_coalesce_pub_upd
  ON knowledge_home_items (user_id, projection_version, COALESCE(published_at, updated_at) DESC)
  WHERE dismissed_at IS NULL AND item_type = 'article';
