-- Migration: optimize feed article indexes
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000013_optimize_feed_article_indexes.up.sql

-- feeds: 並び替えと検索を同時に満たす複合
CREATE INDEX IF NOT EXISTS idx_feeds_created_at_link
    ON feeds (created_at, link);

-- articles: 存在確認だけなので単一キー
CREATE UNIQUE INDEX IF NOT EXISTS idx_articles_url
    ON articles (url);
