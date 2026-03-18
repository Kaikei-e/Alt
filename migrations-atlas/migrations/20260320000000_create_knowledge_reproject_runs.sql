-- Create table for tracking knowledge projection re-projection runs.
-- Reproject allows side-by-side comparison of two projection versions
-- before swapping the active version pointer.
CREATE TABLE knowledge_reproject_runs (
  reproject_run_id    UUID PRIMARY KEY,
  projection_name     TEXT NOT NULL,
  from_version        TEXT NOT NULL,
  to_version          TEXT NOT NULL,
  initiated_by        UUID,
  mode                TEXT NOT NULL, -- full / time_range / user_subset / dry_run
  status              TEXT NOT NULL, -- pending / running / validating / swappable / swapped / failed / cancelled
  range_start         TIMESTAMPTZ,
  range_end           TIMESTAMPTZ,
  checkpoint_payload  JSONB NOT NULL DEFAULT '{}',
  stats_json          JSONB NOT NULL DEFAULT '{}',
  diff_summary_json   JSONB NOT NULL DEFAULT '{}',
  created_at          TIMESTAMPTZ NOT NULL,
  started_at          TIMESTAMPTZ,
  finished_at         TIMESTAMPTZ
);

CREATE INDEX idx_reproject_runs_status ON knowledge_reproject_runs (status, created_at DESC);
