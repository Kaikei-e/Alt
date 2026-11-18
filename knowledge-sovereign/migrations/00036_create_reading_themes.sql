-- Reading themes read models (CQRS projections, rebuildable from knowledge_events).
-- Both tables are disposable projections derived exclusively from event payloads
-- by reading_projector.

CREATE TABLE reading_themes (
  user_id             UUID NOT NULL,
  tenant_id           UUID NOT NULL,
  theme_id            UUID NOT NULL,
  title               TEXT NOT NULL,
  latest_version      INT NOT NULL,
  article_count       INT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL,
  last_opened_at      TIMESTAMPTZ,
  last_opened_version INT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, theme_id)
);

CREATE INDEX idx_reading_themes_user_updated
  ON reading_themes (user_id, updated_at DESC, theme_id);

COMMENT ON TABLE reading_themes IS 'Reading theme summaries (CQRS projection, rebuildable from knowledge_events)';

CREATE TABLE reading_theme_members (
  user_id          UUID NOT NULL,
  tenant_id        UUID NOT NULL,
  theme_id         UUID NOT NULL,
  article_id       UUID NOT NULL,
  added_in_version INT NOT NULL,
  PRIMARY KEY (user_id, theme_id, article_id)
);

CREATE INDEX idx_reading_theme_members_user_article
  ON reading_theme_members (user_id, article_id);

COMMENT ON TABLE reading_theme_members IS 'Articles belonging to each reading theme (CQRS projection, rebuildable from knowledge_events)';
