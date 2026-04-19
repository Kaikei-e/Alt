-- Migration: add language column to articles
-- Atlas Version: v0.35
-- Populated by pre-processor language_detector at ingestion time.
-- Values: BCP-47 short codes ("ja", "en") or "und" when detection is unavailable.

ALTER TABLE articles ADD COLUMN IF NOT EXISTS language TEXT NOT NULL DEFAULT 'und';

CREATE INDEX IF NOT EXISTS idx_articles_language ON articles (language);
