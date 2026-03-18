-- Add feed_ids_json to saved knowledge lens versions for source filtering.
ALTER TABLE knowledge_lens_versions
  ADD COLUMN feed_ids_json JSONB NOT NULL DEFAULT '[]';
