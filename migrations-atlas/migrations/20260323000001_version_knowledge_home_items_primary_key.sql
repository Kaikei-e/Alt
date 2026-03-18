ALTER TABLE knowledge_home_items
  DROP CONSTRAINT knowledge_home_items_pkey;

ALTER TABLE knowledge_home_items
  ADD CONSTRAINT knowledge_home_items_pkey PRIMARY KEY (user_id, item_key, projection_version);

DROP INDEX IF EXISTS idx_kh_items_user_visible_score;

CREATE INDEX IF NOT EXISTS idx_kh_items_user_version_visible_score
  ON knowledge_home_items (user_id, projection_version, score DESC, published_at DESC)
  WHERE dismissed_at IS NULL;
