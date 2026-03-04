-- Create recap_evaluation_runs table for persisting evaluation results
CREATE TABLE IF NOT EXISTS recap_evaluation_runs (
    evaluation_id UUID PRIMARY KEY,
    evaluation_type TEXT NOT NULL,
    job_ids UUID[] NOT NULL DEFAULT '{}',
    metrics JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for efficient history queries ordered by time
CREATE INDEX IF NOT EXISTS idx_recap_evaluation_runs_created_at
    ON recap_evaluation_runs(created_at DESC);

-- Index for filtering by evaluation type
CREATE INDEX IF NOT EXISTS idx_recap_evaluation_runs_type
    ON recap_evaluation_runs(evaluation_type);
