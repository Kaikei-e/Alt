-- Create admin_jobs table for recap-subworker async admin tasks
CREATE TABLE IF NOT EXISTS admin_jobs (
    job_id UUID PRIMARY KEY,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    payload JSONB,
    result JSONB,
    error TEXT
);

-- Quick lookup for in-progress jobs per kind
CREATE INDEX IF NOT EXISTS idx_admin_jobs_kind_status
    ON admin_jobs (kind, status);

