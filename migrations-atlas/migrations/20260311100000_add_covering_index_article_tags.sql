-- Migration: add covering index on article_tags for co-occurrence query optimization
-- The existing idx_article_tags_feed_tag_id only indexes (feed_tag_id).
-- This covering index includes article_id to enable index-only scans
-- when joining article_tags on feed_tag_id and reading article_id.

CREATE INDEX IF NOT EXISTS idx_article_tags_feed_tag_article
    ON article_tags (feed_tag_id, article_id);
