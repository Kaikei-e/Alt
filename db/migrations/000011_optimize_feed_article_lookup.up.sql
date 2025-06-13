-- Add index on articles.url for faster lookups in NOT EXISTS queries
-- (This should already exist due to UNIQUE constraint, but we'll ensure it's optimal)
CREATE INDEX IF NOT EXISTS idx_articles_url_lookup ON articles (url);

-- Add a composite index on feeds for this specific query pattern
CREATE INDEX IF NOT EXISTS idx_feeds_created_at_link ON feeds (created_at ASC, link);

-- Optional: Add a hash index on articles.url for even faster equality lookups
-- (Only if PostgreSQL version supports it and the table is read-heavy)
-- CREATE INDEX IF NOT EXISTS idx_articles_url_hash ON articles USING hash (url);