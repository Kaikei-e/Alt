-- Rename feed_ids_json to source_ids_json so Lens uses source terminology consistently.
ALTER TABLE knowledge_lens_versions
  RENAME COLUMN feed_ids_json TO source_ids_json;
