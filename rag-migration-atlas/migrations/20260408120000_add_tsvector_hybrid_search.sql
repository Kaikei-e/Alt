-- Add tsvector column for in-database BM25-style search (hybrid search with pgvector).
-- Uses a generated column so existing and future chunks get tsvectors automatically.
-- 'english' config for stemmed English, 'simple' config for CJK passthrough.
-- See plan: Phase 2 pgvector Hybrid Search (DB内RRF)

ALTER TABLE rag_chunks ADD COLUMN tsv tsvector
  GENERATED ALWAYS AS (
    to_tsvector('english', content) || to_tsvector('simple', content)
  ) STORED;

-- GIN index for efficient full-text search
CREATE INDEX idx_rag_chunks_tsv ON rag_chunks USING GIN (tsv);
