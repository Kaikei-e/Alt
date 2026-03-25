-- Migration: Partition knowledge_events by occurred_at (monthly)
-- and introduce dedupe registry for global uniqueness.
--
-- Motivation: knowledge_events is append-only and grows without bound.
-- Partitioning enables efficient VACUUM/ANALYZE per partition,
-- future archival via DETACH PARTITION, and BRIN index optimization.
--
-- PostgreSQL constraint: UNIQUE/PK on partitioned tables must include
-- the partition key. We separate global uniqueness (dedupe_key) into
-- a non-partitioned registry table.

--------------------------------------------------------------------------------
-- 1. Create dedupe registry (non-partitioned, global uniqueness)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_event_dedupes (
  dedupe_key    TEXT PRIMARY KEY,
  event_id      UUID NOT NULL,
  occurred_at   TIMESTAMPTZ NOT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE knowledge_event_dedupes
  IS 'Global dedupe registry for knowledge_events idempotency (non-partitioned)';

--------------------------------------------------------------------------------
-- 2. Populate dedupe registry from existing events
--------------------------------------------------------------------------------
INSERT INTO knowledge_event_dedupes (dedupe_key, event_id, occurred_at, first_seen_at)
SELECT dedupe_key, event_id, occurred_at, occurred_at
FROM knowledge_events
ON CONFLICT DO NOTHING;

--------------------------------------------------------------------------------
-- 3. Rename old table, preserve sequence
--------------------------------------------------------------------------------
-- Save the current sequence value
DO $$
DECLARE
  current_val BIGINT;
BEGIN
  SELECT last_value INTO current_val FROM knowledge_events_event_seq_seq;
  PERFORM setval('knowledge_events_event_seq_seq', current_val, true);
END $$;

ALTER TABLE knowledge_events RENAME TO knowledge_events_old;

-- Rename old indexes to avoid name conflicts with new table
ALTER INDEX knowledge_events_pkey RENAME TO knowledge_events_old_pkey;
ALTER INDEX knowledge_events_event_seq_key RENAME TO knowledge_events_old_event_seq_key;
ALTER INDEX knowledge_events_dedupe_key_key RENAME TO knowledge_events_old_dedupe_key_key;
ALTER INDEX idx_knowledge_events_aggregate RENAME TO idx_knowledge_events_old_aggregate;
ALTER INDEX idx_knowledge_events_occurred RENAME TO idx_knowledge_events_old_occurred;
ALTER INDEX idx_knowledge_events_type_seq RENAME TO idx_knowledge_events_old_type_seq;
ALTER INDEX idx_ke_seq_type RENAME TO idx_ke_old_seq_type;

--------------------------------------------------------------------------------
-- 4. Create new partitioned table
--------------------------------------------------------------------------------
CREATE TABLE knowledge_events (
  event_id        UUID NOT NULL DEFAULT gen_random_uuid(),
  event_seq       BIGINT NOT NULL DEFAULT nextval('knowledge_events_event_seq_seq'),
  occurred_at     TIMESTAMPTZ NOT NULL,
  tenant_id       UUID NOT NULL,
  user_id         UUID,
  actor_type      TEXT NOT NULL,
  actor_id        TEXT,
  event_type      TEXT NOT NULL,
  aggregate_type  TEXT NOT NULL,
  aggregate_id    TEXT NOT NULL,
  correlation_id  UUID,
  causation_id    UUID,
  dedupe_key      TEXT NOT NULL,
  payload         JSONB NOT NULL,
  PRIMARY KEY (event_id, occurred_at)
) PARTITION BY RANGE (occurred_at);

-- Reassign sequence ownership to the new table
ALTER SEQUENCE knowledge_events_event_seq_seq OWNED BY knowledge_events.event_seq;

COMMENT ON TABLE knowledge_events
  IS 'Knowledge Home append-only event log (CQRS write side, partitioned by month)';

--------------------------------------------------------------------------------
-- 5. Create monthly partitions (2025-11 through 2026-04)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_events_y2025m11 PARTITION OF knowledge_events
  FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
CREATE TABLE knowledge_events_y2025m12 PARTITION OF knowledge_events
  FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');
CREATE TABLE knowledge_events_y2026m01 PARTITION OF knowledge_events
  FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE knowledge_events_y2026m02 PARTITION OF knowledge_events
  FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE knowledge_events_y2026m03 PARTITION OF knowledge_events
  FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE knowledge_events_y2026m04 PARTITION OF knowledge_events
  FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

-- Default partition for any data outside defined ranges
CREATE TABLE knowledge_events_default PARTITION OF knowledge_events DEFAULT;

--------------------------------------------------------------------------------
-- 6. Migrate data from old table to partitioned table
--------------------------------------------------------------------------------
INSERT INTO knowledge_events (
  event_id, event_seq, occurred_at, tenant_id, user_id,
  actor_type, actor_id, event_type, aggregate_type, aggregate_id,
  correlation_id, causation_id, dedupe_key, payload
)
SELECT
  event_id, event_seq, occurred_at, tenant_id, user_id,
  actor_type, actor_id, event_type, aggregate_type, aggregate_id,
  correlation_id, causation_id, dedupe_key, payload
FROM knowledge_events_old;

--------------------------------------------------------------------------------
-- 7. Re-create indexes on partitioned table
--    (these are created on the parent and inherited by partitions)
--------------------------------------------------------------------------------
CREATE INDEX idx_knowledge_events_aggregate
  ON knowledge_events (aggregate_type, aggregate_id, event_seq);
CREATE INDEX idx_knowledge_events_type_seq
  ON knowledge_events (event_type, event_seq);
CREATE INDEX idx_ke_seq_type
  ON knowledge_events (event_seq, event_type);

-- BRIN index on occurred_at (optimal for append-only, time-correlated data)
-- Keeps existing B-tree for backward compatibility until EXPLAIN confirms BRIN sufficiency
CREATE INDEX idx_knowledge_events_occurred
  ON knowledge_events (occurred_at DESC);
CREATE INDEX idx_knowledge_events_occurred_brin
  ON knowledge_events USING brin (occurred_at) WITH (pages_per_range = 32);

-- event_seq index for projector's ORDER BY event_seq ASC
CREATE INDEX idx_knowledge_events_seq
  ON knowledge_events (event_seq);

--------------------------------------------------------------------------------
-- 8. Drop old table
--------------------------------------------------------------------------------
DROP TABLE knowledge_events_old;
