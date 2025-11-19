-- Recap Worker configuration table (insert-only, latest-wins pattern)
-- Stores Graph Boost thresholds and other tuning parameters learned from genre classification
CREATE TABLE IF NOT EXISTS recap_worker_config (
    id BIGSERIAL PRIMARY KEY,
    config_type TEXT NOT NULL DEFAULT 'graph_override',
    config_payload JSONB NOT NULL,
    source TEXT NOT NULL DEFAULT 'genre_learning',
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for efficient latest config retrieval
CREATE INDEX IF NOT EXISTS idx_recap_worker_config_type_created
    ON recap_worker_config(config_type, created_at DESC);

-- Unique index to ensure only one config_type exists (optional, can be relaxed if needed)
-- We use insert-only pattern, so we query for latest by created_at DESC

-- Comments for documentation
COMMENT ON TABLE recap_worker_config IS 'Insert-only table storing recap-worker configuration updates from genre learning. Query latest by config_type and created_at DESC.';
COMMENT ON COLUMN recap_worker_config.config_type IS 'Type of configuration (e.g., graph_override, refine_config)';
COMMENT ON COLUMN recap_worker_config.config_payload IS 'JSONB object containing configuration values (graph_margin, boost_threshold, etc.)';
COMMENT ON COLUMN recap_worker_config.source IS 'Source of the configuration (genre_learning, manual, etc.)';
COMMENT ON COLUMN recap_worker_config.metadata IS 'Optional metadata about the configuration (accuracy, snapshot info, etc.)';

