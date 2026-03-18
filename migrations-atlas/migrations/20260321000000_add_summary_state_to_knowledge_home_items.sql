-- Add summary_state column to knowledge_home_items.
-- Values: 'missing', 'pending', 'ready'
-- Default 'missing' for existing rows that have no summary excerpt.
-- Rows with a non-empty summary_excerpt are backfilled to 'ready'.
ALTER TABLE knowledge_home_items ADD COLUMN summary_state TEXT NOT NULL DEFAULT 'missing';

-- Backfill: mark items that already have a summary as 'ready'
UPDATE knowledge_home_items SET summary_state = 'ready' WHERE summary_excerpt IS NOT NULL AND summary_excerpt != '';
