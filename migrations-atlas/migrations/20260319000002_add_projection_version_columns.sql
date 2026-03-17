ALTER TABLE knowledge_home_items ADD COLUMN projection_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE today_digest_view ADD COLUMN projection_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE recall_candidate_view ADD COLUMN projection_version INTEGER NOT NULL DEFAULT 1;
CREATE INDEX idx_kh_items_version ON knowledge_home_items (projection_version);
