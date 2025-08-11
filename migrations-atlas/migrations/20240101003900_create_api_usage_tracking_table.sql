-- Migration: create api usage tracking table
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000040_create_api_usage_tracking_table.up.sql

-- Create api_usage_tracking table for monitoring Inoreader API rate limits
CREATE TABLE IF NOT EXISTS api_usage_tracking (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    date DATE DEFAULT CURRENT_DATE,
    zone1_requests INTEGER DEFAULT 0,
    zone2_requests INTEGER DEFAULT 0,
    last_reset TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    rate_limit_headers JSONB DEFAULT '{}'::JSONB
);

-- Create unique constraint to ensure one record per date
ALTER TABLE api_usage_tracking ADD CONSTRAINT uq_api_usage_tracking_date UNIQUE (date);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_api_usage_tracking_date ON api_usage_tracking(date DESC);
CREATE INDEX IF NOT EXISTS idx_api_usage_tracking_last_reset ON api_usage_tracking(last_reset DESC);

-- Add comments for documentation
COMMENT ON TABLE api_usage_tracking IS 'Tracks daily API usage for Inoreader rate limit monitoring (Zone 1: 100/day, Zone 2: 100/day)';
COMMENT ON COLUMN api_usage_tracking.id IS 'Internal UUID primary key';
COMMENT ON COLUMN api_usage_tracking.date IS 'Date for this usage tracking record (YYYY-MM-DD)';
COMMENT ON COLUMN api_usage_tracking.zone1_requests IS 'Number of Zone 1 API requests made (read operations like /subscription/list, /stream/contents)';
COMMENT ON COLUMN api_usage_tracking.zone2_requests IS 'Number of Zone 2 API requests made (write operations like /subscription/edit)';
COMMENT ON COLUMN api_usage_tracking.last_reset IS 'Last time the counters were reset or updated';
COMMENT ON COLUMN api_usage_tracking.rate_limit_headers IS 'JSON object storing rate limit headers from Inoreader API responses';
