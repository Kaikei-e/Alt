CREATE TABLE IF NOT EXISTS public.log_errors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    error_type VARCHAR(255) NOT NULL,
    error_message TEXT,
    raw_line TEXT,
    service VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for efficient time-range queries
CREATE INDEX IF NOT EXISTS idx_log_errors_timestamp ON public.log_errors(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_log_errors_type ON public.log_errors(error_type);
