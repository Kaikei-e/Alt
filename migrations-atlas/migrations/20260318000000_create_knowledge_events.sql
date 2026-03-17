-- Knowledge Home canonical event store (append-only)
CREATE TABLE knowledge_events (
  event_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_seq       BIGSERIAL UNIQUE NOT NULL,
  occurred_at     TIMESTAMPTZ NOT NULL,
  tenant_id       UUID NOT NULL,
  user_id         UUID,
  actor_type      TEXT NOT NULL,
  actor_id        TEXT,
  event_type      TEXT NOT NULL,
  aggregate_type  TEXT NOT NULL,
  aggregate_id    TEXT NOT NULL,
  correlation_id  UUID,
  causation_id    UUID,
  dedupe_key      TEXT NOT NULL UNIQUE,
  payload         JSONB NOT NULL
);

CREATE INDEX idx_knowledge_events_aggregate
  ON knowledge_events (aggregate_type, aggregate_id, event_seq);
CREATE INDEX idx_knowledge_events_occurred
  ON knowledge_events (occurred_at DESC);
CREATE INDEX idx_knowledge_events_type_seq
  ON knowledge_events (event_type, event_seq);

COMMENT ON TABLE knowledge_events IS 'Knowledge Home append-only event log (CQRS write side)';
