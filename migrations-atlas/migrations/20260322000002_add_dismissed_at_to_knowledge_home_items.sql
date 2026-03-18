-- Persist per-user dismissals for Knowledge Home items.
ALTER TABLE knowledge_home_items
  ADD COLUMN dismissed_at TIMESTAMPTZ;

CREATE INDEX idx_kh_items_user_visible_score
  ON knowledge_home_items (user_id, score DESC, published_at DESC)
  WHERE dismissed_at IS NULL;
