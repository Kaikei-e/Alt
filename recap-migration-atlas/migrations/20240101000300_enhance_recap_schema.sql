-- Recap DB schema enhancements
-- Add JSONB indexes and recap_outputs table

-- Ensure sentence_id column exists and constraint enforced
ALTER TABLE recap_subworker_sentences
    ADD COLUMN IF NOT EXISTS sentence_id INT NOT NULL DEFAULT 0;

ALTER TABLE recap_subworker_sentences
    DROP CONSTRAINT IF EXISTS unique_cluster_article_sentence;

ALTER TABLE recap_subworker_sentences
    ADD CONSTRAINT unique_cluster_article_sentence
        UNIQUE (cluster_row_id, source_article_id, sentence_id);

-- Additional GIN indexes
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

-- Recap outputs table
CREATE TABLE IF NOT EXISTS recap_outputs (
    job_id      UUID NOT NULL,
    genre       TEXT NOT NULL,
    response_id TEXT NOT NULL,
    title_ja    TEXT NOT NULL,
    summary_ja  TEXT NOT NULL,
    bullets_ja  JSONB NOT NULL,
    body_json   JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (job_id, genre)
);

CREATE INDEX IF NOT EXISTS idx_recap_outputs_body_json_gin
    ON recap_outputs USING GIN (body_json jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_recap_outputs_response_id
    ON recap_outputs (response_id);

-- Helper function for advisory locks
CREATE OR REPLACE FUNCTION job_lock_key(p_job_id UUID) RETURNS BIGINT AS $$
DECLARE
    hash_text TEXT;
BEGIN
    hash_text := p_job_id::TEXT;
    RETURN ('x' || substring(md5(hash_text), 1, 15))::bit(60)::bigint;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

COMMENT ON TABLE recap_outputs IS 'Final recap outputs with full structured data for UI and search';
COMMENT ON COLUMN recap_outputs.body_json IS 'Complete structured output including clusters, evidence, and metadata';
COMMENT ON FUNCTION job_lock_key IS 'Converts UUID to bigint for use with pg_advisory_lock family';
