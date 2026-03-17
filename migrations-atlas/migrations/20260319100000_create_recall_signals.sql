-- Recall signals track user interactions that feed the recall scoring algorithm.
CREATE TABLE recall_signals (
  signal_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL,
  item_key        TEXT NOT NULL,
  signal_type     TEXT NOT NULL,
  signal_strength NUMERIC NOT NULL DEFAULT 1.0,
  occurred_at     TIMESTAMPTZ NOT NULL,
  payload         JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_recall_signals_user_time ON recall_signals (user_id, occurred_at DESC);
CREATE INDEX idx_recall_signals_item ON recall_signals (user_id, item_key, signal_type);

COMMENT ON TABLE recall_signals IS 'Tracks user interaction signals for recall scoring (Phase 4)';
