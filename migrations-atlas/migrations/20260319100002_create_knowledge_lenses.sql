-- Knowledge Lenses: saved viewpoints for filtering the knowledge stream.
CREATE TABLE knowledge_lenses (
  lens_id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL,
  tenant_id   UUID NOT NULL,
  name        TEXT NOT NULL,
  description TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at TIMESTAMPTZ
);

CREATE INDEX idx_knowledge_lenses_user ON knowledge_lenses (user_id) WHERE archived_at IS NULL;

CREATE TABLE knowledge_lens_versions (
  lens_version_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  lens_id         UUID NOT NULL REFERENCES knowledge_lenses(lens_id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  query_text      TEXT,
  tag_ids_json    JSONB NOT NULL DEFAULT '[]',
  time_window_json JSONB,
  include_recap   BOOLEAN NOT NULL DEFAULT true,
  include_pulse   BOOLEAN NOT NULL DEFAULT true,
  sort_mode       TEXT NOT NULL DEFAULT 'relevance',
  superseded_by   UUID REFERENCES knowledge_lens_versions(lens_version_id)
);

CREATE TABLE knowledge_current_lens (
  user_id         UUID PRIMARY KEY,
  lens_id         UUID NOT NULL REFERENCES knowledge_lenses(lens_id),
  lens_version_id UUID NOT NULL REFERENCES knowledge_lens_versions(lens_version_id),
  selected_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
