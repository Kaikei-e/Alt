-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Use UUID for IDs
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- rag_documents: Manages the current version of a document (article)
CREATE TABLE rag_documents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    article_id TEXT NOT NULL UNIQUE, -- Reference to alt-backend article_id
    current_version_id UUID, -- Will be FK to rag_document_versions, nullable initially
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- rag_document_versions: Immutable versions of a document
CREATE TABLE rag_document_versions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    document_id UUID NOT NULL REFERENCES rag_documents(id) ON DELETE CASCADE,
    version_number INTEGER NOT NULL,
    source_hash TEXT NOT NULL,
    chunker_version TEXT NOT NULL,
    embedder_version TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(document_id, version_number)
);

-- Add the cyclic FK constraint after table creation
ALTER TABLE rag_documents
ADD CONSTRAINT fk_current_version
FOREIGN KEY (current_version_id)
REFERENCES rag_document_versions(id) DEFERRABLE INITIALLY DEFERRED;

-- rag_chunks: Stores the actual text and embeddings for a version
CREATE TABLE rag_chunks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    version_id UUID NOT NULL REFERENCES rag_document_versions(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL,
    content TEXT NOT NULL,
    embedding vector(768), -- Embedding vector
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(version_id, ordinal)
);

-- HNSW Index for vector search
CREATE INDEX rag_chunks_embedding_idx ON rag_chunks USING hnsw (embedding vector_cosine_ops);

-- rag_chunk_events: Tracks changes between versions (added, updated, deleted, unchanged)
-- This provides the "audit trail" or "diff" explanations.
CREATE TYPE chunk_event_type AS ENUM ('added', 'updated', 'deleted', 'unchanged');

CREATE TABLE rag_chunk_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    version_id UUID NOT NULL REFERENCES rag_document_versions(id) ON DELETE CASCADE,
    chunk_id UUID REFERENCES rag_chunks(id), -- Nullable for deleted events if we don't keep the old chunk ref, or if we want to link to specific chunk
    ordinal INTEGER, -- The ordinal in the CURRENT version (for added/updated/unchanged) or PREVIOUS version (for deleted??). Let's assume ordinal in this version.
    event_type chunk_event_type NOT NULL,
    metadata JSONB, -- simplified, can store DEBUG info
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- rag_jobs: Queue for background tasks like backfilling, indexing
CREATE TYPE job_status AS ENUM ('new', 'processing', 'completed', 'failed');

CREATE TABLE rag_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status job_status NOT NULL DEFAULT 'new',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    error_message TEXT
);

-- Index for queue polling
CREATE INDEX rag_jobs_status_created_at_idx ON rag_jobs (status, created_at);
