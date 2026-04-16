-- Composite index supporting the stream subscriber query
--   WHERE event_seq > $1 AND tenant_id = $2 AND (user_id = $3 OR user_id IS NULL)
-- Without it the partitioned knowledge_events table forces a wide scan;
-- (tenant_id, event_seq) lets PostgreSQL satisfy both predicates with a
-- single range scan and check user_id from the heap row.
--
-- Same index also covers the GetLatestKnowledgeEventSeqForUser query:
--   SELECT MAX(event_seq) WHERE tenant_id = $1 AND (user_id = $2 OR user_id IS NULL)

CREATE INDEX IF NOT EXISTS idx_knowledge_events_tenant_seq
  ON knowledge_events (tenant_id, event_seq);
