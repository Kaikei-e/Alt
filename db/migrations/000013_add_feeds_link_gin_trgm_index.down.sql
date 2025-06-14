-- Remove GIN trigram index on feeds.link
DROP INDEX IF EXISTS idx_feeds_link_gin_trgm;

-- Note: We don't drop the pg_trgm extension as it might be used by other indexes or queries