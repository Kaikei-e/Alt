-- Enable pg_trgm extension for trigram similarity operations
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Add GIN trigram index on feeds.link for fast text similarity searches
CREATE INDEX IF NOT EXISTS idx_feeds_link_gin_trgm ON feeds USING gin (link gin_trgm_ops);