-- Add trace context columns to logs table for trace correlation
-- These columns store OpenTelemetry trace_id and span_id from Go services
-- Using FixedString to match otel_logs/otel_traces format

-- Add TraceId column (32-char hex string)
ALTER TABLE logs ADD COLUMN IF NOT EXISTS TraceId FixedString(32) DEFAULT '' AFTER service_group;

-- Add SpanId column (16-char hex string)
ALTER TABLE logs ADD COLUMN IF NOT EXISTS SpanId FixedString(16) DEFAULT '' AFTER TraceId;
