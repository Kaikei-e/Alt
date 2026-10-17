-- Temporarily drop foreign keys on child tables so bulk pruning uses set-based
-- hash joins instead of millions of unindexed per-row FK trigger scans.
ALTER TABLE rag_chunk_events
    DROP CONSTRAINT IF EXISTS rag_chunk_events_chunk_id_fkey,
    DROP CONSTRAINT IF EXISTS rag_chunk_events_version_id_fkey;

ALTER TABLE rag_chunks
    DROP CONSTRAINT IF EXISTS rag_chunks_version_id_fkey;

-- Materialize obsolete version IDs once for set-based cleanup.
CREATE TEMP TABLE _obsolete_versions ON COMMIT DROP AS
SELECT v.id
FROM rag_document_versions v
JOIN rag_documents d ON v.document_id = d.id
WHERE d.current_version_id IS NOT NULL
  AND v.id <> d.current_version_id;

CREATE INDEX ON _obsolete_versions (id);

-- 1. Delete chunk events belonging to obsolete versions.
DELETE FROM rag_chunk_events ce
USING _obsolete_versions ov
WHERE ce.version_id = ov.id;

-- 2. Null out chunk_id references from surviving events (e.g. 'deleted' events
-- in current versions) that point to chunks belonging to obsolete versions.
UPDATE rag_chunk_events ce
SET chunk_id = NULL
FROM rag_chunks c
JOIN _obsolete_versions ov ON c.version_id = ov.id
WHERE ce.chunk_id = c.id;

-- 3. Delete chunks belonging to obsolete versions.
DELETE FROM rag_chunks c
USING _obsolete_versions ov
WHERE c.version_id = ov.id;

-- 4. Delete the obsolete document versions themselves.
DELETE FROM rag_document_versions v
USING _obsolete_versions ov
WHERE v.id = ov.id;

-- Index foreign key columns on rag_chunk_events so runtime DeleteObsoleteVersions
-- cascades/set-nulls via index lookups instead of sequential scans.
CREATE INDEX IF NOT EXISTS idx_rag_chunk_events_version_id
    ON rag_chunk_events (version_id);

CREATE INDEX IF NOT EXISTS idx_rag_chunk_events_chunk_id
    ON rag_chunk_events (chunk_id)
    WHERE chunk_id IS NOT NULL;

-- Restore foreign key constraints (with ON DELETE SET NULL on chunk_id).
ALTER TABLE rag_chunks
    ADD CONSTRAINT rag_chunks_version_id_fkey
        FOREIGN KEY (version_id) REFERENCES rag_document_versions(id) ON DELETE CASCADE;

ALTER TABLE rag_chunk_events
    ADD CONSTRAINT rag_chunk_events_version_id_fkey
        FOREIGN KEY (version_id) REFERENCES rag_document_versions(id) ON DELETE CASCADE,
    ADD CONSTRAINT rag_chunk_events_chunk_id_fkey
        FOREIGN KEY (chunk_id) REFERENCES rag_chunks(id) ON DELETE SET NULL;

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
