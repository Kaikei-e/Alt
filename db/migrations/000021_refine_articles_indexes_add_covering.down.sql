-- Remove the covering index
DROP INDEX IF EXISTS idx_articles_cover_desc;

-- Recreate the original indexes that were dropped
CREATE INDEX IF NOT EXISTS idx_articles_created_id_desc
    ON articles (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_articles_created_at ON articles (created_at);

CREATE INDEX IF NOT EXISTS idx_articles_id_only ON articles (id);
