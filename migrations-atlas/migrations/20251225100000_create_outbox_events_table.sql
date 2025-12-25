-- Create outbox_events table
CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    error_message TEXT
);

-- Index for polling pending events
CREATE INDEX idx_outbox_status_created_at ON outbox_events(status, created_at);

-- Grant permissions to alt_appuser
GRANT SELECT, INSERT, UPDATE ON outbox_events TO alt_appuser;
