-- Performance indexes for Knowledge Home queries.
-- Applied during off-peak hours for large tables.

-- knowledge_home_items: version-aware main query optimization
CREATE INDEX IF NOT EXISTS idx_khi_user_version_score
  ON knowledge_home_items (user_id, projection_version, score DESC, published_at DESC);

-- recall_candidate_view: RecallRail query optimization
CREATE INDEX IF NOT EXISTS idx_rcv_user_version_suggest
  ON recall_candidate_view (user_id, projection_version, next_suggest_at ASC)
  WHERE next_suggest_at IS NOT NULL AND snoozed_until IS NULL;

-- knowledge_user_events: user behavior query optimization
CREATE INDEX IF NOT EXISTS idx_kue_user_occurred
  ON knowledge_user_events (user_id, occurred_at DESC);

-- knowledge_events: aggregate lookup optimization
CREATE INDEX IF NOT EXISTS idx_ke_seq_type
  ON knowledge_events (event_seq, event_type);

-- summary_versions: latest version lookup
CREATE INDEX IF NOT EXISTS idx_sv_article_latest
  ON summary_versions (article_id, generated_at DESC)
  WHERE superseded_by IS NULL;

-- tag_set_versions: latest version lookup
CREATE INDEX IF NOT EXISTS idx_tsv_article_latest
  ON tag_set_versions (article_id, generated_at DESC)
  WHERE superseded_by IS NULL;
