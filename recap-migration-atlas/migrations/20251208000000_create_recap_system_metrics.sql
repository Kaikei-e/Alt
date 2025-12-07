CREATE TABLE IF NOT EXISTS public.recap_system_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID,
    metric_type VARCHAR(50) NOT NULL, -- 'classification', 'clustering', 'summarization'
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metrics JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for faster querying by job_id and type
CREATE INDEX IF NOT EXISTS idx_recap_system_metrics_job_id ON public.recap_system_metrics(job_id);
CREATE INDEX IF NOT EXISTS idx_recap_system_metrics_type_ts ON public.recap_system_metrics(metric_type, timestamp DESC);
