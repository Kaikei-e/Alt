-- Add title and url to rag_document_versions
ALTER TABLE rag_document_versions
ADD COLUMN title TEXT,
ADD COLUMN url TEXT;
