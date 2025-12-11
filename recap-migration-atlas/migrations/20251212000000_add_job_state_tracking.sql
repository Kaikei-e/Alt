-- Add job state tracking columns to recap_jobs
ALTER TABLE recap_jobs
ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending',
ADD COLUMN IF NOT EXISTS last_stage TEXT,
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Create table for logging stage execution details
CREATE TABLE IF NOT EXISTS recap_job_stage_logs (
    id BIGSERIAL PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES recap_jobs(job_id) ON DELETE CASCADE,
    stage TEXT NOT NULL,
    status TEXT NOT NULL, -- 'started', 'completed', 'failed'
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    message TEXT
);

CREATE INDEX IF NOT EXISTS idx_recap_job_stage_logs_job_id ON recap_job_stage_logs(job_id);

-- Create table for storing failed tasks for later retry
CREATE TABLE IF NOT EXISTS recap_failed_tasks (
    id BIGSERIAL PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES recap_jobs(job_id) ON DELETE CASCADE,
    stage TEXT NOT NULL,
    payload JSONB,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recap_failed_tasks_job_id ON recap_failed_tasks(job_id);

-- Create table for checkpointing stage state (JSONB storage)
CREATE TABLE IF NOT EXISTS recap_stage_state (
    job_id UUID NOT NULL REFERENCES recap_jobs(job_id) ON DELETE CASCADE,
    stage TEXT NOT NULL,
    state JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (job_id, stage)
);
