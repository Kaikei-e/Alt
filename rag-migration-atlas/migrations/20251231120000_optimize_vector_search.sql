-- Optimize Vector Search Performance
-- This migration adds indexes to improve the performance of the two-stage vector search.

-- Index on current_version_id for faster JOIN in Stage 2
-- This helps when filtering chunks by current version
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_rag_documents_current_version
ON rag_documents(current_version_id);

-- Index on version_id for faster chunk lookups by version
-- This helps in Stage 2 when enriching chunk data
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_rag_chunks_version_id
ON rag_chunks(version_id);

-- Composite index on document_id and version_id for rag_document_versions
-- This helps speed up the JOIN between versions and documents
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_rag_document_versions_doc_id
ON rag_document_versions(document_id);
