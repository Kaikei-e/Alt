-- Drop feed_tags table and its indexes
DROP INDEX IF EXISTS idx_feed_tags_created_at;
DROP INDEX IF EXISTS idx_feed_tags_tag_id;
DROP TABLE IF EXISTS feed_tags;
