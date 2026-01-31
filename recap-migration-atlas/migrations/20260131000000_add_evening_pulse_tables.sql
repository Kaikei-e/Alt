-- Evening Pulse v4.0 schema additions
-- This migration adds tables for tracking pulse generation, cluster diagnostics, and selection logs.

-- Pulse generation runs with version tracking
CREATE TABLE pulse_generations (
    id              BIGSERIAL PRIMARY KEY,
    job_id          UUID NOT NULL,
    target_date     DATE NOT NULL DEFAULT CURRENT_DATE,
    version         TEXT NOT NULL CHECK (version IN ('v2', 'v3', 'v4')),
    status          TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    topics_count    INT NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ,
    config_snapshot JSONB NOT NULL DEFAULT '{}'::JSONB,
    result_payload  JSONB,
    error_message   TEXT
);

-- Indexes for pulse_generations
CREATE INDEX idx_pulse_generations_job_id
    ON pulse_generations (job_id);

CREATE INDEX idx_pulse_generations_version_status
    ON pulse_generations (version, status);

CREATE INDEX idx_pulse_generations_started_at
    ON pulse_generations (started_at DESC);

CREATE INDEX idx_pulse_generations_target_date
    ON pulse_generations (target_date DESC);

-- GIN index for config_snapshot queries
CREATE INDEX idx_pulse_generations_config_snapshot_gin
    ON pulse_generations USING GIN (config_snapshot jsonb_path_ops);

COMMENT ON TABLE pulse_generations IS 'Evening Pulse generation runs with version tracking';
COMMENT ON COLUMN pulse_generations.job_id IS 'Job UUID for this generation run';
COMMENT ON COLUMN pulse_generations.version IS 'Pulse algorithm version (v2, v3, v4)';
COMMENT ON COLUMN pulse_generations.status IS 'Generation status (running, succeeded, failed)';
COMMENT ON COLUMN pulse_generations.topics_count IS 'Number of topics generated (0-3)';
COMMENT ON COLUMN pulse_generations.config_snapshot IS 'Configuration snapshot at generation time';
COMMENT ON COLUMN pulse_generations.result_payload IS 'Full JSON result for succeeded generations';
COMMENT ON COLUMN pulse_generations.error_message IS 'Error message for failed generations';

-- Per-cluster quality metrics and syndication detection results
CREATE TABLE pulse_cluster_diagnostics (
    id                  BIGSERIAL PRIMARY KEY,
    generation_id       BIGINT NOT NULL REFERENCES pulse_generations(id) ON DELETE CASCADE,
    cluster_id          BIGINT NOT NULL,
    cohesion            REAL NOT NULL,
    ambiguity           REAL NOT NULL,
    entity_consistency  REAL NOT NULL,
    quality_tier        TEXT NOT NULL CHECK (quality_tier IN ('ok', 'caution', 'ng')),
    syndication_status  TEXT CHECK (syndication_status IN ('original', 'canonical_match', 'wire_source', 'title_similar')),
    article_count       INT NOT NULL,
    top_entities        JSONB NOT NULL DEFAULT '[]'::JSONB,
    UNIQUE (generation_id, cluster_id)
);

-- Indexes for pulse_cluster_diagnostics
CREATE INDEX idx_pulse_cluster_diagnostics_generation_id
    ON pulse_cluster_diagnostics (generation_id);

CREATE INDEX idx_pulse_cluster_diagnostics_quality_tier
    ON pulse_cluster_diagnostics (quality_tier);

CREATE INDEX idx_pulse_cluster_diagnostics_syndication
    ON pulse_cluster_diagnostics (syndication_status)
    WHERE syndication_status IS NOT NULL;

-- GIN index for top_entities queries
CREATE INDEX idx_pulse_cluster_diagnostics_top_entities_gin
    ON pulse_cluster_diagnostics USING GIN (top_entities jsonb_path_ops);

COMMENT ON TABLE pulse_cluster_diagnostics IS 'Per-cluster quality metrics and syndication detection results';
COMMENT ON COLUMN pulse_cluster_diagnostics.cohesion IS 'Title cohesion score (0.0-1.0)';
COMMENT ON COLUMN pulse_cluster_diagnostics.ambiguity IS 'Ambiguity score (0.0-1.0, lower is better)';
COMMENT ON COLUMN pulse_cluster_diagnostics.entity_consistency IS 'Entity consistency score (0.0-1.0)';
COMMENT ON COLUMN pulse_cluster_diagnostics.quality_tier IS 'Diagnosed quality tier (ok, caution, ng)';
COMMENT ON COLUMN pulse_cluster_diagnostics.syndication_status IS 'Syndication detection status';
COMMENT ON COLUMN pulse_cluster_diagnostics.top_entities IS 'Top entities extracted from cluster articles';

-- Topic selection decisions with scoring breakdown
CREATE TABLE pulse_selection_log (
    id              BIGSERIAL PRIMARY KEY,
    generation_id   BIGINT NOT NULL REFERENCES pulse_generations(id) ON DELETE CASCADE,
    topic_rank      INT NOT NULL CHECK (topic_rank >= 1 AND topic_rank <= 3),
    cluster_id      BIGINT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('need_to_know', 'trend', 'serendipity')),
    impact_score    REAL NOT NULL,
    burst_score     REAL NOT NULL,
    novelty_score   REAL NOT NULL,
    recency_score   REAL NOT NULL,
    final_score     REAL NOT NULL,
    rationale       TEXT NOT NULL,
    UNIQUE (generation_id, topic_rank)
);

-- Indexes for pulse_selection_log
CREATE INDEX idx_pulse_selection_log_generation_id
    ON pulse_selection_log (generation_id);

CREATE INDEX idx_pulse_selection_log_role
    ON pulse_selection_log (role);

CREATE INDEX idx_pulse_selection_log_cluster_id
    ON pulse_selection_log (cluster_id);

COMMENT ON TABLE pulse_selection_log IS 'Topic selection decisions with scoring breakdown';
COMMENT ON COLUMN pulse_selection_log.topic_rank IS 'Topic rank (1-3)';
COMMENT ON COLUMN pulse_selection_log.role IS 'Assigned role (need_to_know, trend, serendipity)';
COMMENT ON COLUMN pulse_selection_log.impact_score IS 'Impact score component';
COMMENT ON COLUMN pulse_selection_log.burst_score IS 'Burst score component';
COMMENT ON COLUMN pulse_selection_log.novelty_score IS 'Novelty score component';
COMMENT ON COLUMN pulse_selection_log.recency_score IS 'Recency score component';
COMMENT ON COLUMN pulse_selection_log.final_score IS 'Final weighted score';
COMMENT ON COLUMN pulse_selection_log.rationale IS 'Human-readable rationale for selection';

-- View for latest pulse generation per job
CREATE OR REPLACE VIEW pulse_latest_generations AS
SELECT DISTINCT ON (job_id)
    id,
    job_id,
    version,
    status,
    topics_count,
    started_at,
    finished_at,
    config_snapshot,
    result_payload,
    error_message
FROM pulse_generations
ORDER BY job_id, started_at DESC;

COMMENT ON VIEW pulse_latest_generations IS 'Latest pulse generation for each job';

-- View for pulse quality statistics
CREATE OR REPLACE VIEW pulse_quality_stats AS
SELECT
    pg.version,
    pcd.quality_tier,
    COUNT(*) as cluster_count,
    AVG(pcd.cohesion) as avg_cohesion,
    AVG(pcd.ambiguity) as avg_ambiguity,
    AVG(pcd.entity_consistency) as avg_entity_consistency,
    AVG(pcd.article_count) as avg_article_count
FROM pulse_generations pg
JOIN pulse_cluster_diagnostics pcd ON pg.id = pcd.generation_id
WHERE pg.status = 'succeeded'
GROUP BY pg.version, pcd.quality_tier
ORDER BY pg.version, pcd.quality_tier;

COMMENT ON VIEW pulse_quality_stats IS 'Quality statistics by version and tier';

-- View for syndication removal statistics
CREATE OR REPLACE VIEW pulse_syndication_stats AS
SELECT
    pg.version,
    pcd.syndication_status,
    COUNT(*) as cluster_count
FROM pulse_generations pg
JOIN pulse_cluster_diagnostics pcd ON pg.id = pcd.generation_id
WHERE pg.status = 'succeeded'
  AND pcd.syndication_status IS NOT NULL
GROUP BY pg.version, pcd.syndication_status
ORDER BY pg.version, pcd.syndication_status;

COMMENT ON VIEW pulse_syndication_stats IS 'Syndication detection statistics by version';
