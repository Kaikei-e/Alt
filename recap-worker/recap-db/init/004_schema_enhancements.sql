-- recap-worker/recap-db/init/004_schema_enhancements.sql
-- Schema enhancements: add missing sentence_id, JSONB GIN indexes, and recap_outputs table

-- Add sentence_id column to recap_subworker_sentences (used by DAO but missing in schema)
ALTER TABLE recap_subworker_sentences
ADD COLUMN IF NOT EXISTS sentence_id INT NOT NULL DEFAULT 0;

-- Add unique constraint for sentence identification
ALTER TABLE recap_subworker_sentences
DROP CONSTRAINT IF EXISTS unique_cluster_article_sentence;

ALTER TABLE recap_subworker_sentences
ADD CONSTRAINT unique_cluster_article_sentence
    UNIQUE (cluster_row_id, source_article_id, sentence_id);

-- JSONB GIN indexes for efficient querying
-- Using jsonb_path_ops for containment queries (@>, @@, @?) - smaller and faster
CREATE INDEX IF NOT EXISTS idx_recap_subworker_runs_request_payload_gin
    ON recap_subworker_runs USING GIN (request_payload jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_recap_subworker_runs_response_payload_gin
    ON recap_subworker_runs USING GIN (response_payload jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_recap_subworker_clusters_top_terms_gin
    ON recap_subworker_clusters USING GIN (top_terms jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_recap_subworker_clusters_stats_gin
    ON recap_subworker_clusters USING GIN (stats jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_recap_subworker_diagnostics_value_gin
    ON recap_subworker_diagnostics USING GIN (value jsonb_path_ops);

-- New table for final recap outputs (separate from legacy recap_sections)
CREATE TABLE IF NOT EXISTS recap_outputs (
    job_id      UUID NOT NULL,
    genre       TEXT NOT NULL,
    response_id TEXT NOT NULL,
    title_ja    TEXT NOT NULL,
    summary_ja  TEXT NOT NULL,
    bullets_ja  JSONB NOT NULL,  -- [{"text":"…","sources":[["a1",17],…]}, …]
    body_json   JSONB NOT NULL,  -- Full structured output for search/analysis
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (job_id, genre)
);

-- GIN index for body_json searchability
CREATE INDEX IF NOT EXISTS idx_recap_outputs_body_json_gin
    ON recap_outputs USING GIN (body_json jsonb_path_ops);

-- Index for response_id lookups
CREATE INDEX IF NOT EXISTS idx_recap_outputs_response_id
    ON recap_outputs (response_id);

-- Helper function for job locking (converts UUID to bigint for advisory locks)
CREATE OR REPLACE FUNCTION job_lock_key(p_job_id UUID) RETURNS BIGINT AS $$
DECLARE
    hash_text TEXT;
BEGIN
    -- Convert UUID to text and hash to bigint range
    hash_text := p_job_id::TEXT;
    RETURN ('x' || substring(md5(hash_text), 1, 15))::bit(60)::bigint;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Comments for documentation
COMMENT ON TABLE recap_outputs IS 'Final recap outputs with full structured data for UI and search';
COMMENT ON COLUMN recap_outputs.body_json IS 'Complete structured output including clusters, evidence, and metadata';
COMMENT ON FUNCTION job_lock_key IS 'Converts UUID to bigint for use with pg_advisory_lock family';

