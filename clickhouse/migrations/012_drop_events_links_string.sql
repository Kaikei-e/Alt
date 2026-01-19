-- Migration: Drop Events and Links String columns from otel_traces
-- Reason: These columns conflict with Events.* and Links.* nested arrays when using clickhouse crate
-- The nested array columns (Events.Timestamp, Events.Name, etc.) are used by Grafana ClickHouse datasource

ALTER TABLE otel_traces DROP COLUMN IF EXISTS Events;
ALTER TABLE otel_traces DROP COLUMN IF EXISTS Links;
