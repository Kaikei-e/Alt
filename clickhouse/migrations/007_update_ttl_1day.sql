-- TTL Retention Policy Update: Reduce all tables to 1 day retention
-- Purpose: Reduce disk consumption by shortening log retention period
-- Previous values: logs/http_logs: 2 days, otel_logs/otel_traces: 7 days
--
-- Best practices applied:
-- - ttl_only_drop_parts=1 is already set on all tables
-- - Partition-aligned TTL for efficient deletion
--
-- Created: 2026-01-14

ALTER TABLE logs MODIFY TTL timestamp + INTERVAL 1 DAY DELETE;
ALTER TABLE http_logs MODIFY TTL timestamp + INTERVAL 1 DAY DELETE;
ALTER TABLE otel_logs MODIFY TTL Timestamp + INTERVAL 1 DAY DELETE;
ALTER TABLE otel_traces MODIFY TTL Timestamp + INTERVAL 1 DAY DELETE;
