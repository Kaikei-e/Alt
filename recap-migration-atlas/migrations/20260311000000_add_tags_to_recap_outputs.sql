-- Add semantic tags column to recap_outputs for tag-generator enriched tags.
-- Existing top_terms (from c-TF-IDF) are preserved in recap_subworker_clusters.
-- This new column stores KeyBERT-based semantic tags per genre output.
ALTER TABLE recap_outputs ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_recap_outputs_tags_gin ON recap_outputs USING GIN (tags jsonb_path_ops);
