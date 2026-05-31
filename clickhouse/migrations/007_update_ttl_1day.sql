-- TTL Retention Policy Update: Reduce all log/trace tables to 1 day retention
-- Purpose: Reduce disk consumption by shortening log retention period
-- Previous values: logs/http_logs: 2 days, otel_logs/otel_traces: 7 days,
--                  otel_http_requests: 7 days, otel_error_logs: 14 days
--
-- Best practices applied:
-- - ttl_only_drop_parts=1 is already set on all tables
-- - Partition-aligned TTL for efficient deletion
--
-- sli_metrics is intentionally excluded: it keeps its 90-day window (009) for
-- SLO trend analysis and is tiny (minute-granularity aggregates).
--
-- Created: 2026-01-14

ALTER TABLE logs MODIFY TTL timestamp + INTERVAL 1 DAY DELETE;
ALTER TABLE http_logs MODIFY TTL timestamp + INTERVAL 1 DAY DELETE;
ALTER TABLE otel_logs MODIFY TTL Timestamp + INTERVAL 1 DAY DELETE;
ALTER TABLE otel_traces MODIFY TTL Timestamp + INTERVAL 1 DAY DELETE;

-- Derived tables created in 006 were missed by the original policy update.
ALTER TABLE otel_http_requests MODIFY TTL Timestamp + INTERVAL 1 DAY DELETE;
ALTER TABLE otel_error_logs MODIFY TTL Timestamp + INTERVAL 1 DAY DELETE;
