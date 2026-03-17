-- Versioned summary artifacts (immutable except superseded_by)
CREATE TABLE summary_versions (
  summary_version_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  article_id         UUID NOT NULL,
  user_id            UUID NOT NULL,
  generated_at       TIMESTAMPTZ NOT NULL,
  model              TEXT NOT NULL,
  prompt_version     TEXT NOT NULL,
  input_hash         TEXT NOT NULL,
  quality_score      NUMERIC,
  summary_text       TEXT NOT NULL,
  superseded_by      UUID
);

CREATE INDEX idx_summary_versions_article
  ON summary_versions (article_id, generated_at DESC);

COMMENT ON TABLE summary_versions IS 'Versioned summary artifacts (immutable except superseded_by)';
