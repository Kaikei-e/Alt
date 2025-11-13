-- Genre learning results table for two-stage classification
-- Stores coarse candidates, refine decisions, tag profiles, and telemetry
CREATE TABLE IF NOT EXISTS recap_genre_learning_results (
    job_id UUID NOT NULL,
    article_id TEXT NOT NULL,
    coarse_candidates JSONB NOT NULL,
    refine_decision JSONB NOT NULL,
    tag_profile JSONB NOT NULL,
    graph_context JSONB NOT NULL DEFAULT '[]'::JSONB,
    feedback JSONB,
    telemetry JSONB,
    timestamps JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (job_id, article_id)
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_recap_genre_learning_results_job_id
    ON recap_genre_learning_results(job_id);

CREATE INDEX IF NOT EXISTS idx_recap_genre_learning_results_article_id
    ON recap_genre_learning_results(article_id);

-- GIN index for JSONB path queries on refine_decision
CREATE INDEX IF NOT EXISTS idx_recap_genre_learning_results_refine_decision_gin
    ON recap_genre_learning_results USING GIN (refine_decision jsonb_path_ops);

-- GIN index for JSONB path queries on tag_profile
CREATE INDEX IF NOT EXISTS idx_recap_genre_learning_results_tag_profile_gin
    ON recap_genre_learning_results USING GIN (tag_profile jsonb_path_ops);

-- GIN index for JSONB path queries on coarse_candidates
CREATE INDEX IF NOT EXISTS idx_recap_genre_learning_results_coarse_candidates_gin
    ON recap_genre_learning_results USING GIN (coarse_candidates jsonb_path_ops);

-- Comments for documentation
COMMENT ON TABLE recap_genre_learning_results IS 'Stores genre classification learning records including coarse candidates, refine decisions, and tag profiles for evaluation and model improvement';
COMMENT ON COLUMN recap_genre_learning_results.job_id IS 'Foreign key to recap_jobs.job_id';
COMMENT ON COLUMN recap_genre_learning_results.article_id IS 'Source article identifier';
COMMENT ON COLUMN recap_genre_learning_results.coarse_candidates IS 'JSONB array of coarse stage candidates with scores and keyword support';
COMMENT ON COLUMN recap_genre_learning_results.refine_decision IS 'JSONB object containing final genre, confidence, strategy, and LLM trace ID';
COMMENT ON COLUMN recap_genre_learning_results.tag_profile IS 'JSONB object containing top tags, entropy, and tag signals from Tag Generator';
COMMENT ON COLUMN recap_genre_learning_results.graph_context IS 'JSONB array of graph edges used during refinement';
COMMENT ON COLUMN recap_genre_learning_results.feedback IS 'Optional JSONB object for manual feedback and corrections';
COMMENT ON COLUMN recap_genre_learning_results.telemetry IS 'Optional JSONB object containing performance metrics (latency, cache hits, etc.)';
COMMENT ON COLUMN recap_genre_learning_results.timestamps IS 'JSONB object containing coarse/refine stage timestamps';

