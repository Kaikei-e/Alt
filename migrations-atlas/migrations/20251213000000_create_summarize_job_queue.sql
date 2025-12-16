-- Migration: create summarize job queue table
-- Created: 2025-12-13 00:00:00
-- Atlas Version: v0.35
-- Description: Creates a queue table for asynchronous article summarization jobs

-- Create summarize_job_queue table for managing async summarization requests
CREATE TABLE IF NOT EXISTS summarize_job_queue (
    id SERIAL PRIMARY KEY,
    job_id UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    article_id TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    summary TEXT,
    error_message TEXT,
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

-- Indexes for efficient querying
-- Index for finding pending/running jobs (used by worker)
CREATE INDEX IF NOT EXISTS idx_summarize_job_queue_status
    ON summarize_job_queue(status)
    WHERE status IN ('pending', 'running');

-- Index for job lookup by job_id (used by status endpoint)
CREATE INDEX IF NOT EXISTS idx_summarize_job_queue_job_id
    ON summarize_job_queue(job_id);

-- Index for article lookup (used for duplicate prevention)
CREATE INDEX IF NOT EXISTS idx_summarize_job_queue_article_id
    ON summarize_job_queue(article_id);

-- Add comments for documentation
COMMENT ON TABLE summarize_job_queue IS 'Queue table for asynchronous article summarization jobs';
COMMENT ON COLUMN summarize_job_queue.id IS 'Internal serial primary key';
COMMENT ON COLUMN summarize_job_queue.job_id IS 'Unique UUID identifier for the job (returned to client)';
COMMENT ON COLUMN summarize_job_queue.article_id IS 'Article ID (TEXT) to be summarized';
COMMENT ON COLUMN summarize_job_queue.status IS 'Job status: pending, running, completed, failed';
COMMENT ON COLUMN summarize_job_queue.summary IS 'Generated summary (populated when status is completed)';
COMMENT ON COLUMN summarize_job_queue.error_message IS 'Error message (populated when status is failed)';
COMMENT ON COLUMN summarize_job_queue.retry_count IS 'Number of retry attempts';
COMMENT ON COLUMN summarize_job_queue.max_retries IS 'Maximum number of retry attempts allowed';
COMMENT ON COLUMN summarize_job_queue.created_at IS 'Timestamp when job was created';
COMMENT ON COLUMN summarize_job_queue.started_at IS 'Timestamp when job processing started';
COMMENT ON COLUMN summarize_job_queue.completed_at IS 'Timestamp when job processing completed';

-- Grant permissions to preprocessor user
GRANT SELECT, INSERT, UPDATE ON TABLE summarize_job_queue TO pre_processor_user;

-- Grant usage on sequence
GRANT USAGE, SELECT ON SEQUENCE summarize_job_queue_id_seq TO pre_processor_user;

