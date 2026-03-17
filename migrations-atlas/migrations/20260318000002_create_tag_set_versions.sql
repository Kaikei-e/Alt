-- Versioned tag set snapshots (immutable except superseded_by)
CREATE TABLE tag_set_versions (
  tag_set_version_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  article_id         UUID NOT NULL,
  user_id            UUID NOT NULL,
  generated_at       TIMESTAMPTZ NOT NULL,
  generator          TEXT NOT NULL,
  input_hash         TEXT NOT NULL,
  tags_json          JSONB NOT NULL,
  superseded_by      UUID
);

CREATE INDEX idx_tag_set_versions_article
  ON tag_set_versions (article_id, generated_at DESC);

COMMENT ON TABLE tag_set_versions IS 'Versioned tag set snapshots (immutable except superseded_by)';
