-- Add supersede tracking to knowledge_home_items for version change UX.
ALTER TABLE knowledge_home_items
  ADD COLUMN IF NOT EXISTS supersede_state TEXT,
  ADD COLUMN IF NOT EXISTS superseded_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS previous_ref_json JSONB;

CREATE INDEX idx_kh_items_supersede
  ON knowledge_home_items (user_id, supersede_state) WHERE supersede_state IS NOT NULL;
