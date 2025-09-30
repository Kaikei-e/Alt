-- Rollback: Restore the original covering index with content field
-- Note: This may cause index size errors for articles with large content

DROP INDEX IF EXISTS idx_articles_cover_desc;

CREATE INDEX IF NOT EXISTS idx_articles_cover_desc
  ON articles (created_at DESC, id DESC)
  INCLUDE (title, content, url);