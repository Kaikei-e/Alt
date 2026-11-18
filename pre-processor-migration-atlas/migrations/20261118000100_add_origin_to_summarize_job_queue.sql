-- Keep DEFAULT 'system' so rolling-deploy instances on the prior binary can still INSERT without specifying origin.
ALTER TABLE summarize_job_queue
    ADD COLUMN origin TEXT NOT NULL DEFAULT 'system'
    CONSTRAINT summarize_job_queue_origin_check CHECK (origin IN ('user', 'system'));
