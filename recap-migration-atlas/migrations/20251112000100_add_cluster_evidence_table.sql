-- Deduplicated evidence table per cluster
CREATE TABLE IF NOT EXISTS recap_cluster_evidence (
    id BIGSERIAL PRIMARY KEY,
    cluster_row_id BIGINT NOT NULL REFERENCES recap_subworker_clusters(id) ON DELETE CASCADE,
    article_id TEXT NOT NULL,
    title TEXT,
    source_url TEXT,
    published_at TIMESTAMPTZ,
    lang TEXT,
    rank SMALLINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uniq_recap_cluster_evidence_article
    ON recap_cluster_evidence (cluster_row_id, article_id);

CREATE INDEX IF NOT EXISTS idx_recap_cluster_evidence_cluster_rank
    ON recap_cluster_evidence (cluster_row_id, rank);

CREATE INDEX IF NOT EXISTS idx_recap_cluster_evidence_article
    ON recap_cluster_evidence (article_id);
