-- Classification job queue table for persistent job management
-- This table stores classification chunks that are queued for processing
-- by recap-worker, allowing for retry and recovery from worker restarts

CREATE TABLE IF NOT EXISTS classification_job_queue (
    id SERIAL PRIMARY KEY,
    recap_job_id UUID NOT NULL,
    chunk_idx INT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'retrying')),
    texts JSONB NOT NULL,
    result JSONB,
    error_message TEXT,
    retry_count INT DEFAULT 0,
    max_retries INT DEFAULT 3,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    UNIQUE(recap_job_id, chunk_idx)
);

-- Index for efficient status-based queries (used by workers to pick next job)
CREATE INDEX IF NOT EXISTS idx_classification_queue_status
    ON classification_job_queue(status)
    WHERE status IN ('pending', 'retrying');

-- Index for job-level queries
CREATE INDEX IF NOT EXISTS idx_classification_queue_job_id
    ON classification_job_queue(recap_job_id);

-- Composite index for efficient job status queries
CREATE INDEX IF NOT EXISTS idx_classification_queue_job_status
    ON classification_job_queue(recap_job_id, status);

