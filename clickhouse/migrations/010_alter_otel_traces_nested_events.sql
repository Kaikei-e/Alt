-- Add nested columns for Events and Links to support Grafana ClickHouse datasource
-- The Grafana ClickHouse plugin expects these columns for OTel trace visualization
--
-- Created: 2026-01-18
-- Updated: 2026-01-29 - Merged with migration 012 to handle column name conflicts
-- Purpose: Fix "Unknown expression or function identifier 'Events.Name'" error in Grafana
--
-- Note: First drop old String columns (Events, Links) that conflict with nested column names,
-- then add the nested columns following OpenTelemetry spec and Grafana expectations.

-- Step 1: Drop old String columns that conflict with nested column naming
-- ClickHouse doesn't allow both "Events" (String) and "Events.Timestamp" (Array) to coexist
ALTER TABLE otel_traces DROP COLUMN IF EXISTS Events;
ALTER TABLE otel_traces DROP COLUMN IF EXISTS Links;

-- Step 2: Add nested Events columns
ALTER TABLE otel_traces
    ADD COLUMN IF NOT EXISTS `Events.Timestamp` Array(DateTime64(9, 'UTC')) DEFAULT [] CODEC(ZSTD(1)),
    ADD COLUMN IF NOT EXISTS `Events.Name` Array(LowCardinality(String)) DEFAULT [] CODEC(ZSTD(1)),
    ADD COLUMN IF NOT EXISTS `Events.Attributes` Array(Map(LowCardinality(String), String)) DEFAULT [] CODEC(ZSTD(1));

-- Step 3: Add nested Links columns
ALTER TABLE otel_traces
    ADD COLUMN IF NOT EXISTS `Links.TraceId` Array(String) DEFAULT [] CODEC(ZSTD(1)),
    ADD COLUMN IF NOT EXISTS `Links.SpanId` Array(String) DEFAULT [] CODEC(ZSTD(1)),
    ADD COLUMN IF NOT EXISTS `Links.TraceState` Array(String) DEFAULT [] CODEC(ZSTD(1)),
    ADD COLUMN IF NOT EXISTS `Links.Attributes` Array(Map(LowCardinality(String), String)) DEFAULT [] CODEC(ZSTD(1));
