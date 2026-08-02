-- Widen rag_chunks.embedding from vector(768) (embeddinggemma) to vector(1024)
-- (bge-m3).
--
-- The stored vectors are discarded rather than converted: a 768-dim
-- embeddinggemma vector has no meaning in bge-m3's space, so there is no cast
-- that preserves it. rag_chunks is a disposable projection — the embeddings
-- are rebuilt from article text by the backfill playbook, and the append-only
-- rag_document_versions / rag_chunk_events history is untouched.
--
-- The HNSW index is dropped first because pgvector cannot alter the dimension
-- of an indexed vector column; rebuilding it after the rewrite is also the
-- documented bulk-load order.

DROP INDEX IF EXISTS rag_chunks_embedding_idx;

ALTER TABLE rag_chunks
    ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);

CREATE INDEX rag_chunks_embedding_idx ON rag_chunks USING hnsw (embedding vector_cosine_ops);
