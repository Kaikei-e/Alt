-- Composite index supporting the producer-liveness gauge probe
--   SELECT occurred_at FROM knowledge_events
--   WHERE event_type = $1 ORDER BY occurred_at DESC LIMIT 1
-- run once per watched event type every ProjectionHealthTickInterval.
--
-- No existing index pairs these columns: idx_knowledge_events_type_seq is
-- (event_type, event_seq) and idx_knowledge_events_occurred is (occurred_at)
-- alone, so the probe would otherwise walk the whole log for a producer that
-- has stopped emitting — precisely the case ADR-000939 relies on this gauge to
-- catch. With (event_type, occurred_at DESC) each partition answers with a
-- single index descent and MergeAppend stops after the first row.
--
-- Leading column is event_type because the probe pins it to equality; the
-- trailing DESC matches the scan direction so no extra sort is planned.

CREATE INDEX IF NOT EXISTS idx_knowledge_events_type_occurred
  ON knowledge_events (event_type, occurred_at DESC);

COMMENT ON INDEX idx_knowledge_events_type_occurred
  IS 'Producer-liveness gauge: newest occurred_at per event_type in one index descent per partition';
