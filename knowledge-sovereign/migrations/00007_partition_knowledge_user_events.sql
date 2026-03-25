-- Migration: Partition knowledge_user_events by occurred_at (monthly)
--
-- Same motivation as knowledge_events partitioning.
-- dedupe_key is nullable with a partial unique index, so no separate
-- dedupe registry is needed — the partial index works on partitioned tables
-- as long as dedupe_key + occurred_at are both in the constraint.

--------------------------------------------------------------------------------
-- 1. Rename old table
--------------------------------------------------------------------------------
ALTER TABLE knowledge_user_events RENAME TO knowledge_user_events_old;

-- Rename old indexes/constraints to avoid name conflicts
ALTER INDEX knowledge_user_events_pkey RENAME TO knowledge_user_events_old_pkey;
ALTER INDEX idx_knowledge_user_events_user_time RENAME TO idx_kue_old_user_time;
ALTER INDEX uq_knowledge_user_events_dedupe_key RENAME TO uq_kue_old_dedupe_key;
ALTER INDEX idx_kue_user_occurred RENAME TO idx_kue_old_occurred;

--------------------------------------------------------------------------------
-- 2. Create new partitioned table
--------------------------------------------------------------------------------
CREATE TABLE knowledge_user_events (
  user_event_id  UUID NOT NULL DEFAULT gen_random_uuid(),
  occurred_at    TIMESTAMPTZ NOT NULL,
  user_id        UUID NOT NULL,
  tenant_id      UUID NOT NULL,
  event_type     TEXT NOT NULL,
  item_key       TEXT NOT NULL,
  payload        JSONB NOT NULL DEFAULT '{}',
  dedupe_key     TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_event_id, occurred_at)
) PARTITION BY RANGE (occurred_at);

COMMENT ON TABLE knowledge_user_events
  IS 'User interaction log for Knowledge Home (partitioned by month)';

--------------------------------------------------------------------------------
-- 3. Create monthly partitions (2025-11 through 2026-04)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_user_events_y2025m11 PARTITION OF knowledge_user_events
  FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
CREATE TABLE knowledge_user_events_y2025m12 PARTITION OF knowledge_user_events
  FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');
CREATE TABLE knowledge_user_events_y2026m01 PARTITION OF knowledge_user_events
  FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE knowledge_user_events_y2026m02 PARTITION OF knowledge_user_events
  FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE knowledge_user_events_y2026m03 PARTITION OF knowledge_user_events
  FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE knowledge_user_events_y2026m04 PARTITION OF knowledge_user_events
  FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE knowledge_user_events_default PARTITION OF knowledge_user_events DEFAULT;

--------------------------------------------------------------------------------
-- 4. Migrate data
--------------------------------------------------------------------------------
INSERT INTO knowledge_user_events (
  user_event_id, occurred_at, user_id, tenant_id,
  event_type, item_key, payload, dedupe_key
)
SELECT
  user_event_id, occurred_at, user_id, tenant_id,
  event_type, item_key, payload, dedupe_key
FROM knowledge_user_events_old;

--------------------------------------------------------------------------------
-- 5. Re-create indexes
--------------------------------------------------------------------------------
CREATE INDEX idx_knowledge_user_events_user_time
  ON knowledge_user_events (user_id, occurred_at DESC);
CREATE INDEX idx_kue_user_occurred
  ON knowledge_user_events (user_id, occurred_at DESC);

-- UNIQUE constraint on dedupe_key must include partition key (occurred_at)
-- for partitioned tables per PostgreSQL requirements.
CREATE UNIQUE INDEX uq_knowledge_user_events_dedupe_key
  ON knowledge_user_events (dedupe_key, occurred_at)
  WHERE dedupe_key != '';

-- BRIN index for time-range queries
CREATE INDEX idx_knowledge_user_events_occurred_brin
  ON knowledge_user_events USING brin (occurred_at) WITH (pages_per_range = 32);

--------------------------------------------------------------------------------
-- 6. Drop old table
--------------------------------------------------------------------------------
DROP TABLE knowledge_user_events_old;
