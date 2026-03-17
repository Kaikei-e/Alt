-- Projector checkpoint for incremental event processing
CREATE TABLE knowledge_projection_checkpoints (
  projector_name TEXT PRIMARY KEY,
  last_event_seq BIGINT NOT NULL DEFAULT 0,
  updated_at     TIMESTAMPTZ NOT NULL
);

COMMENT ON TABLE knowledge_projection_checkpoints IS 'Projector checkpoint for incremental event processing';
