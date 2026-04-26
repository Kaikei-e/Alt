-- Knowledge Loop why_text backfill (ADR-000846): the knowledge_backfill_jobs
-- table now hosts more than one backfill stream. The kind column lets the
-- alt-backend job runner discriminate between the original article-replay
-- stream ('articles') and the new summary-narrative repair stream
-- ('summary_narratives').
--
-- Default 'articles' preserves the semantics of every existing row so the
-- KnowledgeBackfillJob continues to walk articles unchanged. The new
-- SummaryNarrativeBackfillJob inserts rows with kind='summary_narratives'.
ALTER TABLE knowledge_backfill_jobs
  ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'articles';

-- Index supports the runner's "next pending or running job for this kind"
-- query: it's the hot path on each scheduler tick.
CREATE INDEX IF NOT EXISTS idx_knowledge_backfill_jobs_kind_status
  ON knowledge_backfill_jobs (kind, status, created_at);
