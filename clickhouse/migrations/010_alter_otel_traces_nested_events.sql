-- Add nested columns for Events and Links to support Grafana ClickHouse datasource
-- The Grafana ClickHouse plugin expects these columns for OTel trace visualization
--
-- Created: 2026-01-18
-- Purpose: Fix "Unknown expression or function identifier 'Events.Name'" error in Grafana
--
-- Note: The existing Events and Links String columns are kept for backward compatibility.
-- New nested columns follow OpenTelemetry spec and Grafana expectations.

-- Add nested Events columns
ALTER TABLE otel_traces
    ADD COLUMN IF NOT EXISTS `Events.Timestamp` Array(DateTime64(9, 'UTC')) DEFAULT [] CODEC(ZSTD(1)),
    ADD COLUMN IF NOT EXISTS `Events.Name` Array(LowCardinality(String)) DEFAULT [] CODEC(ZSTD(1)),
    ADD COLUMN IF NOT EXISTS `Events.Attributes` Array(Map(LowCardinality(String), String)) DEFAULT [] CODEC(ZSTD(1));

-- Add nested Links columns
ALTER TABLE otel_traces
    ADD COLUMN IF NOT EXISTS `Links.TraceId` Array(String) DEFAULT [] CODEC(ZSTD(1)),
    ADD COLUMN IF NOT EXISTS `Links.SpanId` Array(String) DEFAULT [] CODEC(ZSTD(1)),
    ADD COLUMN IF NOT EXISTS `Links.TraceState` Array(String) DEFAULT [] CODEC(ZSTD(1)),
    ADD COLUMN IF NOT EXISTS `Links.Attributes` Array(Map(LowCardinality(String), String)) DEFAULT [] CODEC(ZSTD(1));
