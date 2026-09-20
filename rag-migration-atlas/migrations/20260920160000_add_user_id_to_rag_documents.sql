-- Add nullable user_id to rag_documents for multi-tenant / per-user scoping
ALTER TABLE rag_documents ADD COLUMN user_id UUID;

-- Index for per-user document retrieval and scoping
CREATE INDEX idx_rag_documents_user_id ON rag_documents (user_id);
