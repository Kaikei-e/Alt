CREATE TABLE knowledge_backfill_jobs (
  job_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  status              TEXT NOT NULL DEFAULT 'pending',
  projection_version  INTEGER NOT NULL,
  cursor_user_id      UUID,
  cursor_date         DATE,
  cursor_article_id   UUID,
  total_events        INTEGER NOT NULL DEFAULT 0,
  processed_events    INTEGER NOT NULL DEFAULT 0,
  error_message       TEXT,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at          TIMESTAMPTZ,
  completed_at        TIMESTAMPTZ,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_backfill_jobs_status ON knowledge_backfill_jobs (status);
