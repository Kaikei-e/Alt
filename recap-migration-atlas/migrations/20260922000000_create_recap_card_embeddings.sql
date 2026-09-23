-- A deterministic cache of embedding vectors keyed by the input text hash and model identity.
-- Written with INSERT ... ON CONFLICT DO NOTHING (never updated), so calibration replays
-- of the same window do not re-embed. Rows are not job-scoped and are excluded from the
-- recap_jobs retention cascade by construction (no FK).

CREATE TABLE recap_card_embeddings (
    text_hash  TEXT NOT NULL,          -- xxh3-64 hex of the exact embedding input text
    model      TEXT NOT NULL,          -- embedding identity, e.g. bge-m3
    dim        INT  NOT NULL,
    embedding  REAL[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (text_hash, model)
);
