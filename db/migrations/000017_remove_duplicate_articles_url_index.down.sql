-- Recreate the articles URL lookup index (in case rollback is needed)
CREATE INDEX IF NOT EXISTS idx_articles_url_lookup ON articles (url);