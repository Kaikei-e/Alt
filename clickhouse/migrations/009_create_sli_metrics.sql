-- SLI Metrics Table and Materialized Views
-- Purpose: Derive Service Level Indicators from logs for observability
--
-- Created: 2026-01-15

-- SLI metrics storage table
CREATE TABLE IF NOT EXISTS sli_metrics (
    Timestamp DateTime CODEC(DoubleDelta, ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    Metric LowCardinality(String) CODEC(ZSTD(1)),
    Value Float64 CODEC(ZSTD(1)),
    Tags Map(LowCardinality(String), String) CODEC(ZSTD(1))
)
ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, Metric, Timestamp)
TTL Timestamp + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

-- Error rate per service (1-minute granularity)
CREATE MATERIALIZED VIEW IF NOT EXISTS sli_error_rate_mv
TO sli_metrics
AS SELECT
    toStartOfMinute(src.Timestamp) AS Timestamp,
    ServiceName,
    'error_rate' AS Metric,
    countIf(SeverityNumber >= 17) / count() AS Value,
    map('window', '1m') AS Tags
FROM otel_logs AS src
WHERE src.Timestamp > now() - INTERVAL 1 HOUR
GROUP BY ServiceName, Timestamp;

-- Log throughput per service (1-minute granularity)
CREATE MATERIALIZED VIEW IF NOT EXISTS sli_log_throughput_mv
TO sli_metrics
AS SELECT
    toStartOfMinute(src.Timestamp) AS Timestamp,
    ServiceName,
    'log_throughput' AS Metric,
    count() AS Value,
    map('window', '1m') AS Tags
FROM otel_logs AS src
WHERE src.Timestamp > now() - INTERVAL 1 HOUR
GROUP BY ServiceName, Timestamp;
