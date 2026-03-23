-- Add link column to knowledge_home_items for self-contained reads (no articles JOIN needed).
ALTER TABLE knowledge_home_items ADD COLUMN IF NOT EXISTS link TEXT NOT NULL DEFAULT '';
