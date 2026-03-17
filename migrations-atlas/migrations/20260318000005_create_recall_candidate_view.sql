-- Recall candidates projection (Phase 1: skeleton only)
CREATE TABLE recall_candidate_view (
  user_id        UUID NOT NULL,
  item_key       TEXT NOT NULL,
  recall_score   NUMERIC NOT NULL DEFAULT 0,
  reason_json    JSONB NOT NULL DEFAULT '[]',
  next_suggest_at TIMESTAMPTZ,
  updated_at     TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (user_id, item_key)
);

COMMENT ON TABLE recall_candidate_view IS 'Recall candidates projection (Phase 1: skeleton only)';
