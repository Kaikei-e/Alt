-- Micro-externalization & theme co-creation read models (ADR-001044).
-- Disposable CQRS projections rebuilt exclusively from knowledge_events by reading_projector.

CREATE TABLE reading_annotations (
  user_id        UUID NOT NULL,
  tenant_id      UUID NOT NULL,
  annotation_id  UUID NOT NULL,
  article_id     UUID NOT NULL,
  stance         TEXT NOT NULL,
  excerpt        TEXT NOT NULL DEFAULT '',
  note           TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL,
  deleted_at     TIMESTAMPTZ,
  last_event_seq BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, annotation_id)
);

CREATE INDEX idx_reading_annotations_user_article_active
  ON reading_annotations (user_id, article_id, created_at DESC)
  WHERE deleted_at IS NULL;

CREATE INDEX idx_reading_annotations_user_recent_active
  ON reading_annotations (user_id, created_at DESC)
  WHERE deleted_at IS NULL;

COMMENT ON TABLE reading_annotations IS 'User article micro-externalizations (CQRS projection, rebuildable from knowledge_events)';

CREATE TABLE reading_theme_curations (
  user_id               UUID NOT NULL,
  tenant_id             UUID NOT NULL,
  curation_id           UUID NOT NULL,
  theme_id              UUID NOT NULL,
  action_kind           TEXT NOT NULL,
  target_hash           TEXT NOT NULL DEFAULT '',
  target_text           TEXT NOT NULL DEFAULT '',
  user_note             TEXT NOT NULL DEFAULT '',
  augur_conversation_id TEXT NOT NULL DEFAULT '',
  created_at            TIMESTAMPTZ NOT NULL,
  deleted_at            TIMESTAMPTZ,
  last_event_seq        BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, curation_id)
);

CREATE INDEX idx_reading_theme_curations_user_theme_active
  ON reading_theme_curations (user_id, theme_id, created_at DESC)
  WHERE deleted_at IS NULL;

COMMENT ON TABLE reading_theme_curations IS 'User co-creation actions on reading themes (CQRS projection, rebuildable from knowledge_events)';
