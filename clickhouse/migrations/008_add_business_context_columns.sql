-- Business Context Materialized Columns for otel_logs
-- Purpose: Extract Alt-specific business context from LogAttributes for efficient querying
-- Semantic attributes follow OpenTelemetry conventions with 'alt.' prefix
--
-- Created: 2026-01-15

-- Add materialized columns that automatically extract business context from LogAttributes
-- These are computed on INSERT and stored for fast querying

-- Feed tracking
ALTER TABLE otel_logs ADD COLUMN IF NOT EXISTS FeedId String
    MATERIALIZED LogAttributes['alt.feed.id'] CODEC(ZSTD(1));

-- Article tracking
ALTER TABLE otel_logs ADD COLUMN IF NOT EXISTS ArticleId String
    MATERIALIZED LogAttributes['alt.article.id'] CODEC(ZSTD(1));

-- Job tracking (recap, rag, etc.)
ALTER TABLE otel_logs ADD COLUMN IF NOT EXISTS JobId String
    MATERIALIZED LogAttributes['alt.job.id'] CODEC(ZSTD(1));

-- Processing stage tracking
ALTER TABLE otel_logs ADD COLUMN IF NOT EXISTS ProcessingStage LowCardinality(String)
    MATERIALIZED LogAttributes['alt.processing.stage'] CODEC(ZSTD(1));

-- AI pipeline identification
ALTER TABLE otel_logs ADD COLUMN IF NOT EXISTS AIPipeline LowCardinality(String)
    MATERIALIZED LogAttributes['alt.ai.pipeline'] CODEC(ZSTD(1));

-- Request correlation ID
ALTER TABLE otel_logs ADD COLUMN IF NOT EXISTS RequestId String
    MATERIALIZED LogAttributes['alt.request.id'] CODEC(ZSTD(1));

-- Add bloom filter indexes for efficient lookups on IDs
ALTER TABLE otel_logs ADD INDEX IF NOT EXISTS idx_feed_id FeedId TYPE bloom_filter(0.01) GRANULARITY 1;
ALTER TABLE otel_logs ADD INDEX IF NOT EXISTS idx_article_id ArticleId TYPE bloom_filter(0.01) GRANULARITY 1;
ALTER TABLE otel_logs ADD INDEX IF NOT EXISTS idx_job_id JobId TYPE bloom_filter(0.01) GRANULARITY 1;
ALTER TABLE otel_logs ADD INDEX IF NOT EXISTS idx_request_id RequestId TYPE bloom_filter(0.01) GRANULARITY 1;
