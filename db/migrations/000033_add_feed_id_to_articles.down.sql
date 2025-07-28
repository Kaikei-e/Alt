-- Remove the index on feed_id
DROP INDEX IF EXISTS idx_articles_feed_id;

-- Remove the foreign key constraint
ALTER TABLE articles
    DROP CONSTRAINT IF EXISTS fk_articles_feed_id;

-- Remove the feed_id column from the articles table
ALTER TABLE articles
    DROP COLUMN IF EXISTS feed_id;