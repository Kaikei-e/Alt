-- Morning Letter v2: document-first morning briefing persistence
-- target_date is Asia/Tokyo based (edition_timezone records this explicitly)

CREATE TABLE morning_letters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    target_date DATE NOT NULL,
    edition_timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Tokyo',
    source_recap_job_id UUID,
    is_degraded BOOLEAN NOT NULL DEFAULT FALSE,
    schema_version INTEGER NOT NULL DEFAULT 1,
    generation_revision INTEGER NOT NULL DEFAULT 1,
    result_jsonb JSONB NOT NULL,
    model VARCHAR(100),
    generation_metadata_jsonb JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_morning_letters_target_date ON morning_letters(target_date, edition_timezone);
CREATE INDEX idx_morning_letters_created_at ON morning_letters(created_at DESC);

CREATE TABLE morning_letter_sources (
    letter_id UUID NOT NULL REFERENCES morning_letters(id) ON DELETE CASCADE,
    section_key VARCHAR(100) NOT NULL,
    article_id UUID NOT NULL,
    source_type VARCHAR(20) NOT NULL DEFAULT 'overnight',
    position INTEGER NOT NULL,
    PRIMARY KEY (letter_id, section_key, article_id)
);

CREATE INDEX idx_morning_letter_sources_article ON morning_letter_sources(article_id);
