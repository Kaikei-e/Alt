-- Knowledge Home read model (CQRS projection, rebuildable)
CREATE TABLE knowledge_home_items (
  user_id            UUID NOT NULL,
  tenant_id          UUID NOT NULL,
  item_key           TEXT NOT NULL,
  item_type          TEXT NOT NULL,
  primary_ref_id     UUID,
  title              TEXT NOT NULL,
  summary_excerpt    TEXT,
  tags_json          JSONB NOT NULL DEFAULT '[]',
  why_json           JSONB NOT NULL DEFAULT '[]',
  score              NUMERIC NOT NULL DEFAULT 0,
  freshness_at       TIMESTAMPTZ,
  published_at       TIMESTAMPTZ,
  last_interacted_at TIMESTAMPTZ,
  generated_at       TIMESTAMPTZ NOT NULL,
  updated_at         TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (user_id, item_key)
);

CREATE INDEX idx_kh_items_user_score
  ON knowledge_home_items (user_id, score DESC, published_at DESC);

COMMENT ON TABLE knowledge_home_items IS 'Knowledge Home read model (CQRS projection, rebuildable)';
