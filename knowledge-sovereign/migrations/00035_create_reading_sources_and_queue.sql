-- Reading-first read models (CQRS projections, rebuildable from knowledge_events).
-- Both tables are disposable projections derived exclusively from event payloads
-- and event occurred_at by reading_projector.

CREATE TABLE reading_sources (
  user_id           UUID NOT NULL,
  tenant_id         UUID NOT NULL,
  article_id        UUID NOT NULL,
  first_read_at     TIMESTAMPTZ NOT NULL,
  read_rule_version INT NOT NULL,
  PRIMARY KEY (user_id, article_id)
);

CREATE INDEX idx_reading_sources_user_first_read
  ON reading_sources (user_id, first_read_at DESC)
  WHERE read_rule_version >= 1;

COMMENT ON TABLE reading_sources IS 'Articles that counted as read (CQRS projection, rebuildable from knowledge_events)';

CREATE TABLE reading_queue_items (
  user_id              UUID NOT NULL,
  tenant_id            UUID NOT NULL,
  article_id           UUID NOT NULL,
  queued_at            TIMESTAMPTZ NOT NULL,
  expires_at           TIMESTAMPTZ NOT NULL,
  queue_policy_version INT NOT NULL,
  removed_at           TIMESTAMPTZ,
  removed_reason       TEXT NOT NULL DEFAULT '',
  last_event_seq       BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, article_id)
);

-- Active queue lookup filters removed_at IS NULL AND expires_at > as_of,
-- ordered by queued_at ASC.
CREATE INDEX idx_reading_queue_items_user_active
  ON reading_queue_items (user_id, expires_at, queued_at ASC)
  WHERE removed_at IS NULL;

COMMENT ON TABLE reading_queue_items IS 'Read Today queue read model (CQRS projection, rebuildable from knowledge_events)';
