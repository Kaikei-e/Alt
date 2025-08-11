-- Migration: create feed tags table
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000025_create_feed_tags_table.up.sql

-- Create the feed_tags junction table
CREATE TABLE feed_tags (
    feed_id    UUID NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    tag_id     INT  NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (feed_id, tag_id)
);

ALTER TABLE feed_tags
    OWNER TO alt_db_user;

CREATE INDEX idx_feed_tags_tag_id
    ON feed_tags (tag_id);

CREATE INDEX idx_feed_tags_created_at
    ON feed_tags (created_at);
