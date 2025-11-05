-- recap-worker/recap-db/init/001_base_schema.sql
-- Base schema for recap worker persistence.

CREATE TABLE IF NOT EXISTS recap_subworker_runs (
    id              BIGSERIAL PRIMARY KEY,
    job_id          UUID NOT NULL,
    genre           TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'partial', 'failed')),
    cluster_count   INT NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ,
    request_payload JSONB NOT NULL DEFAULT '{}'::JSONB,
    response_payload JSONB,
    error_message   TEXT
);

CREATE INDEX IF NOT EXISTS idx_recap_subworker_runs_job_id
    ON recap_subworker_runs (job_id);

CREATE TABLE IF NOT EXISTS recap_subworker_clusters (
    id          BIGSERIAL PRIMARY KEY,
    run_id      BIGINT NOT NULL REFERENCES recap_subworker_runs(id) ON DELETE CASCADE,
    cluster_id  INT NOT NULL,
    size        INT NOT NULL,
    label       TEXT,
    top_terms   JSONB NOT NULL,
    stats       JSONB NOT NULL,
    UNIQUE (run_id, cluster_id)
);

CREATE INDEX IF NOT EXISTS idx_recap_subworker_clusters_run_id
    ON recap_subworker_clusters (run_id);

CREATE TABLE IF NOT EXISTS recap_subworker_sentences (
    id                BIGSERIAL PRIMARY KEY,
    cluster_row_id    BIGINT NOT NULL REFERENCES recap_subworker_clusters(id) ON DELETE CASCADE,
    source_article_id TEXT NOT NULL,
    paragraph_idx     INT,
    sentence_text     TEXT NOT NULL,
    lang              TEXT NOT NULL DEFAULT 'unknown',
    score             REAL NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_recap_subworker_sentences_cluster_row_id
    ON recap_subworker_sentences (cluster_row_id);

CREATE TABLE IF NOT EXISTS recap_subworker_diagnostics (
    run_id  BIGINT NOT NULL REFERENCES recap_subworker_runs(id) ON DELETE CASCADE,
    metric  TEXT NOT NULL,
    value   JSONB NOT NULL,
    PRIMARY KEY (run_id, metric)
);

-- Legacy table used by the recap pipeline for compatibility.
CREATE TABLE IF NOT EXISTS recap_sections (
    job_id      UUID NOT NULL,
    genre       TEXT NOT NULL,
    response_id TEXT,
    PRIMARY KEY (job_id, genre)
);
