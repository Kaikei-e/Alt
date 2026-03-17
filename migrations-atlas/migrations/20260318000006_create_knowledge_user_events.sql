-- User interaction log for Knowledge Home
CREATE TABLE knowledge_user_events (
  user_event_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  occurred_at    TIMESTAMPTZ NOT NULL,
  user_id        UUID NOT NULL,
  tenant_id      UUID NOT NULL,
  event_type     TEXT NOT NULL,
  item_key       TEXT NOT NULL,
  payload        JSONB NOT NULL DEFAULT '{}',
  dedupe_key     TEXT
);

CREATE INDEX idx_knowledge_user_events_user_time
  ON knowledge_user_events (user_id, occurred_at DESC);
CREATE UNIQUE INDEX idx_knowledge_user_events_dedupe
  ON knowledge_user_events (dedupe_key) WHERE dedupe_key IS NOT NULL;

COMMENT ON TABLE knowledge_user_events IS 'User interaction log for Knowledge Home';
