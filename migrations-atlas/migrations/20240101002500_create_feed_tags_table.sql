-- Migration: create feed tags table
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000025_create_feed_tags_table.up.sql

-- Create the feed_tags table (this is actually the main tags table)
CREATE TABLE feed_tags (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    feed_id    UUID NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    tag_name   TEXT NOT NULL,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    tag_type   VARCHAR(50) DEFAULT 'auto'
);

ALTER TABLE feed_tags
    OWNER TO alt_db_user;

CREATE INDEX idx_feed_tags_feed_id
    ON feed_tags (feed_id);

CREATE INDEX idx_feed_tags_tag_name
    ON feed_tags (tag_name);

CREATE INDEX idx_feed_tags_confidence
    ON feed_tags (confidence DESC);

CREATE INDEX idx_feed_tags_created_at
    ON feed_tags (created_at);
