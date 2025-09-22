-- Drop redundant indexes that will be covered by the new covering index
DROP INDEX IF EXISTS idx_articles_created_id_desc;
DROP INDEX IF EXISTS idx_articles_created_at;
DROP INDEX IF EXISTS idx_articles_id_only;

-- Create the new covering index that includes frequently accessed columns
-- This covers queries that need created_at/id ordering plus title, content, url access
CREATE INDEX IF NOT EXISTS idx_articles_cover_desc
  ON articles (created_at DESC, id DESC)
  INCLUDE (title, content, url);
