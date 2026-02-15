-- Migration: initial pre-processor-db schema
-- Created: 2026-02-15
-- Description: Consolidated creation of all 5 pre-processor tables
--   migrated from alt-db (ADR-000246)

-- 1. inoreader_subscriptions
CREATE TABLE IF NOT EXISTS inoreader_subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inoreader_id TEXT UNIQUE NOT NULL,
    feed_url TEXT NOT NULL,
    title TEXT,
    category TEXT,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inoreader_subscriptions_inoreader_id ON inoreader_subscriptions(inoreader_id);
CREATE INDEX IF NOT EXISTS idx_inoreader_subscriptions_feed_url ON inoreader_subscriptions(feed_url);
CREATE INDEX IF NOT EXISTS idx_inoreader_subscriptions_synced_at ON inoreader_subscriptions(synced_at DESC);

COMMENT ON TABLE inoreader_subscriptions IS 'Stores RSS feed subscriptions synchronized from Inoreader API';
COMMENT ON COLUMN inoreader_subscriptions.id IS 'Internal UUID primary key';
COMMENT ON COLUMN inoreader_subscriptions.inoreader_id IS 'Unique identifier from Inoreader API (e.g., feed/http://example.com/rss)';
COMMENT ON COLUMN inoreader_subscriptions.feed_url IS 'XML RSS feed URL';
COMMENT ON COLUMN inoreader_subscriptions.title IS 'Feed title from Inoreader';
COMMENT ON COLUMN inoreader_subscriptions.category IS 'Feed category/folder from Inoreader';
COMMENT ON COLUMN inoreader_subscriptions.synced_at IS 'Last synchronization timestamp';
COMMENT ON COLUMN inoreader_subscriptions.created_at IS 'Record creation timestamp';

-- 2. inoreader_articles (depends on inoreader_subscriptions)
CREATE TABLE IF NOT EXISTS inoreader_articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inoreader_id TEXT UNIQUE NOT NULL,
    subscription_id UUID REFERENCES inoreader_subscriptions(id) ON DELETE CASCADE,
    article_url TEXT NOT NULL,
    title TEXT,
    author TEXT,
    published_at TIMESTAMP WITH TIME ZONE,
    fetched_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processed BOOLEAN DEFAULT FALSE,
    content TEXT,
    content_length INTEGER DEFAULT 0,
    content_type VARCHAR(50) DEFAULT 'html'
);

CREATE INDEX IF NOT EXISTS idx_inoreader_articles_inoreader_id ON inoreader_articles(inoreader_id);
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_subscription_id ON inoreader_articles(subscription_id);
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_article_url ON inoreader_articles(article_url);
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_published_at ON inoreader_articles(published_at DESC);
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_fetched_at ON inoreader_articles(fetched_at DESC);
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_processed ON inoreader_articles(processed) WHERE processed = FALSE;
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_has_content ON inoreader_articles(content_length) WHERE content_length > 0;
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_processed_content ON inoreader_articles(processed, content_length) WHERE content_length > 0;
CREATE INDEX IF NOT EXISTS idx_inoreader_articles_content_type ON inoreader_articles(content_type) WHERE content_type IS NOT NULL AND content_type != 'html';

COMMENT ON TABLE inoreader_articles IS 'Stores article metadata fetched from Inoreader stream contents API';
COMMENT ON COLUMN inoreader_articles.id IS 'Internal UUID primary key';
COMMENT ON COLUMN inoreader_articles.inoreader_id IS 'Unique article identifier from Inoreader API';
COMMENT ON COLUMN inoreader_articles.subscription_id IS 'Reference to inoreader_subscriptions table';
COMMENT ON COLUMN inoreader_articles.article_url IS 'URL to the original article';
COMMENT ON COLUMN inoreader_articles.title IS 'Article title from Inoreader';
COMMENT ON COLUMN inoreader_articles.author IS 'Article author';
COMMENT ON COLUMN inoreader_articles.published_at IS 'Original publication timestamp';
COMMENT ON COLUMN inoreader_articles.fetched_at IS 'When this record was fetched from Inoreader';
COMMENT ON COLUMN inoreader_articles.processed IS 'Whether this article has been processed by other services';
COMMENT ON COLUMN inoreader_articles.content IS 'Full article content from Inoreader summary.content field';
COMMENT ON COLUMN inoreader_articles.content_length IS 'Length of content in characters for optimization';
COMMENT ON COLUMN inoreader_articles.content_type IS 'Content type (html, html_rtl, text)';

-- 3. sync_state
CREATE TABLE IF NOT EXISTS sync_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stream_id TEXT UNIQUE NOT NULL,
    continuation_token TEXT,
    last_sync TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sync_state_stream_id ON sync_state(stream_id);
CREATE INDEX IF NOT EXISTS idx_sync_state_last_sync ON sync_state(last_sync DESC);
CREATE INDEX IF NOT EXISTS idx_sync_state_created_at ON sync_state(created_at DESC);

COMMENT ON TABLE sync_state IS 'Stores synchronization state and continuation tokens for Inoreader stream pagination';
COMMENT ON COLUMN sync_state.id IS 'Internal UUID primary key';
COMMENT ON COLUMN sync_state.stream_id IS 'Stream identifier (e.g., user/-/state/com.google/reading-list)';
COMMENT ON COLUMN sync_state.continuation_token IS 'Continuation token for pagination from Inoreader API';
COMMENT ON COLUMN sync_state.last_sync IS 'Last successful synchronization timestamp';
COMMENT ON COLUMN sync_state.created_at IS 'Timestamp when the sync state record was first created';

-- 4. api_usage_tracking
CREATE TABLE IF NOT EXISTS api_usage_tracking (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    date DATE DEFAULT CURRENT_DATE,
    zone1_requests INTEGER DEFAULT 0,
    zone2_requests INTEGER DEFAULT 0,
    last_reset TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    rate_limit_headers JSONB DEFAULT '{}'::JSONB
);

ALTER TABLE api_usage_tracking ADD CONSTRAINT uq_api_usage_tracking_date UNIQUE (date);

CREATE INDEX IF NOT EXISTS idx_api_usage_tracking_date ON api_usage_tracking(date DESC);
CREATE INDEX IF NOT EXISTS idx_api_usage_tracking_last_reset ON api_usage_tracking(last_reset DESC);

COMMENT ON TABLE api_usage_tracking IS 'Tracks daily API usage for Inoreader rate limit monitoring (Zone 1: 100/day, Zone 2: 100/day)';
COMMENT ON COLUMN api_usage_tracking.id IS 'Internal UUID primary key';
COMMENT ON COLUMN api_usage_tracking.date IS 'Date for this usage tracking record (YYYY-MM-DD)';
COMMENT ON COLUMN api_usage_tracking.zone1_requests IS 'Number of Zone 1 API requests made (read operations)';
COMMENT ON COLUMN api_usage_tracking.zone2_requests IS 'Number of Zone 2 API requests made (write operations)';
COMMENT ON COLUMN api_usage_tracking.last_reset IS 'Last time the counters were reset or updated';
COMMENT ON COLUMN api_usage_tracking.rate_limit_headers IS 'JSON object storing rate limit headers from Inoreader API responses';

-- 5. summarize_job_queue
CREATE TABLE IF NOT EXISTS summarize_job_queue (
    id SERIAL PRIMARY KEY,
    job_id UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    article_id TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    summary TEXT,
    error_message TEXT,
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_summarize_job_queue_status ON summarize_job_queue(status) WHERE status IN ('pending', 'running');
CREATE INDEX IF NOT EXISTS idx_summarize_job_queue_job_id ON summarize_job_queue(job_id);
CREATE INDEX IF NOT EXISTS idx_summarize_job_queue_article_id ON summarize_job_queue(article_id);

COMMENT ON TABLE summarize_job_queue IS 'Queue table for asynchronous article summarization jobs';
COMMENT ON COLUMN summarize_job_queue.id IS 'Internal serial primary key';
COMMENT ON COLUMN summarize_job_queue.job_id IS 'Unique UUID identifier for the job (returned to client)';
COMMENT ON COLUMN summarize_job_queue.article_id IS 'Article ID (TEXT) to be summarized';
COMMENT ON COLUMN summarize_job_queue.status IS 'Job status: pending, running, completed, failed';
COMMENT ON COLUMN summarize_job_queue.summary IS 'Generated summary (populated when status is completed)';
COMMENT ON COLUMN summarize_job_queue.error_message IS 'Error message (populated when status is failed)';
COMMENT ON COLUMN summarize_job_queue.retry_count IS 'Number of retry attempts';
COMMENT ON COLUMN summarize_job_queue.max_retries IS 'Maximum number of retry attempts allowed';
COMMENT ON COLUMN summarize_job_queue.created_at IS 'Timestamp when job was created';
COMMENT ON COLUMN summarize_job_queue.started_at IS 'Timestamp when job processing started';
COMMENT ON COLUMN summarize_job_queue.completed_at IS 'Timestamp when job processing completed';
