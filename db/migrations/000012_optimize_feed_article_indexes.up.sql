-- feeds: 並び替えと検索を同時に満たす複合
CREATE INDEX IF NOT EXISTS idx_feeds_created_at_link
    ON feeds (created_at, link);

-- articles: 存在確認だけなので単一キー
CREATE UNIQUE INDEX IF NOT EXISTS idx_articles_url
    ON articles (url);