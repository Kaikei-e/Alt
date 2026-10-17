-- Allow deleting superseded chunks even when a newer version's deleted event
-- still references the old chunk_id.
ALTER TABLE rag_chunk_events
    DROP CONSTRAINT IF EXISTS rag_chunk_events_chunk_id_fkey,
    ADD CONSTRAINT rag_chunk_events_chunk_id_fkey
        FOREIGN KEY (chunk_id) REFERENCES rag_chunks(id) ON DELETE SET NULL;

-- Delete superseded document versions (cascades to rag_chunks and rag_chunk_events).
DELETE FROM rag_document_versions v
USING rag_documents d
WHERE v.document_id = d.id
  AND d.current_version_id IS NOT NULL
  AND v.id <> d.current_version_id;

-- Partial index for fast missing-embedding coverage checks without heap vector reads.
CREATE INDEX IF NOT EXISTS idx_rag_chunks_missing_embedding
    ON rag_chunks (version_id)
    WHERE embedding IS NULL;

-- Prune old terminal jobs and index active queue entries for AcquireNextJob.
DELETE FROM rag_jobs
WHERE status IN ('completed', 'failed')
  AND updated_at < NOW() - INTERVAL '7 days';

CREATE INDEX IF NOT EXISTS idx_rag_jobs_active_created_at
    ON rag_jobs (created_at ASC)
    WHERE status IN ('new', 'processing');
