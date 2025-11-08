-- Recap DB final sections
CREATE TABLE IF NOT EXISTS recap_final_sections (
    job_id      UUID NOT NULL,
    genre       TEXT NOT NULL,
    response_id TEXT NOT NULL,
    title_ja    TEXT NOT NULL,
    summary_ja  TEXT NOT NULL,
    bullets_ja  JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (job_id, genre)
);
