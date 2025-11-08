-- Recap DB job tracking tables
CREATE TABLE IF NOT EXISTS recap_jobs (
    id         BIGSERIAL PRIMARY KEY,
    job_id     UUID NOT NULL UNIQUE,
    kicked_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note       TEXT
);

CREATE TABLE IF NOT EXISTS recap_job_articles (
    id             BIGSERIAL PRIMARY KEY,
    job_id         UUID NOT NULL REFERENCES recap_jobs(job_id) ON DELETE CASCADE,
    article_id     TEXT NOT NULL,
    title          TEXT,
    fulltext_html  TEXT NOT NULL,
    published_at   TIMESTAMPTZ,
    source_url     TEXT,
    lang_hint      TEXT,
    normalized_hash TEXT NOT NULL,
    UNIQUE (job_id, article_id)
);

CREATE INDEX IF NOT EXISTS idx_recap_job_articles_job
    ON recap_job_articles (job_id);

CREATE TABLE IF NOT EXISTS recap_preprocess_metrics (
    job_id   UUID NOT NULL REFERENCES recap_jobs(job_id) ON DELETE CASCADE,
    metric   TEXT NOT NULL,
    value    JSONB NOT NULL,
    PRIMARY KEY (job_id, metric)
);
