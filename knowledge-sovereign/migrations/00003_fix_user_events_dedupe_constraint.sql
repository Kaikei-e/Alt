-- Replace partial unique index with a full UNIQUE constraint on dedupe_key.
-- dedupe_key must be NOT NULL for ON CONFLICT (dedupe_key) to work.

ALTER TABLE knowledge_user_events ALTER COLUMN dedupe_key SET NOT NULL;
ALTER TABLE knowledge_user_events ALTER COLUMN dedupe_key SET DEFAULT '';

DROP INDEX IF EXISTS idx_knowledge_user_events_dedupe;

ALTER TABLE knowledge_user_events ADD CONSTRAINT uq_knowledge_user_events_dedupe_key UNIQUE (dedupe_key);
