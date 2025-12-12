-- Add recap_run_diagnostics table for typed cluster statistics
-- This table stores run-level aggregated statistics from cluster avg_sim values
-- for easier querying and reporting compared to the KV-style recap_subworker_diagnostics
-- Note: This table is typically written once per run completion and not updated.
-- The run_id references recap_subworker_runs.finished_at for timing information.

CREATE TABLE IF NOT EXISTS recap_run_diagnostics (
    run_id                          BIGINT NOT NULL PRIMARY KEY REFERENCES recap_subworker_runs(id) ON DELETE CASCADE,
    cluster_avg_similarity_mean    DOUBLE PRECISION NULL,
    cluster_avg_similarity_variance DOUBLE PRECISION NULL,
    cluster_avg_similarity_p95     DOUBLE PRECISION NULL,
    cluster_avg_similarity_max     DOUBLE PRECISION NULL,
    cluster_count                  INT NOT NULL DEFAULT 0,
    created_at                     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recap_run_diagnostics_run_id
    ON recap_run_diagnostics (run_id);

