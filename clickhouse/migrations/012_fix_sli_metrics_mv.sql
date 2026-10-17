-- Fix SLI Metrics Materialized Views to process only newly inserted blocks
-- Removing the table alias (AS src) and 1-hour lookback WHERE clause prevents
-- ClickHouse from rescanning existing otel_logs parts on every 5-second batch insert.

DROP VIEW IF EXISTS sli_error_rate_mv;
DROP VIEW IF EXISTS sli_log_throughput_mv;

CREATE MATERIALIZED VIEW IF NOT EXISTS sli_error_rate_mv
TO sli_metrics
AS SELECT
    toStartOfMinute(Timestamp) AS Timestamp,
    ServiceName,
    'error_rate' AS Metric,
    countIf(SeverityNumber >= 17) / count() AS Value,
    map('window', '1m') AS Tags
FROM otel_logs
GROUP BY ServiceName, Timestamp;

CREATE MATERIALIZED VIEW IF NOT EXISTS sli_log_throughput_mv
TO sli_metrics
AS SELECT
    toStartOfMinute(Timestamp) AS Timestamp,
    ServiceName,
    'log_throughput' AS Metric,
    count() AS Value,
    map('window', '1m') AS Tags
FROM otel_logs
GROUP BY ServiceName, Timestamp;
