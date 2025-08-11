-- Migration: fix tags schema
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000030_fix_tags_schema.up.sql

-- Fix the tags schema
-- The feed_tags table is actually the main tags table with id and name
-- We need to rename it and fix the article_tags table structure

-- 1. Rename feed_tags to tags (since it has the correct structure: id, name)
ALTER TABLE feed_tags RENAME TO tags;

-- 2. Rename the sequence as well
ALTER SEQUENCE feed_tags_id_seq RENAME TO tags_id_seq;

-- 3. Update the primary key constraint name
ALTER TABLE tags RENAME CONSTRAINT feed_tags_pkey TO tags_pkey;

-- 4. Update the unique constraint name
ALTER TABLE tags RENAME CONSTRAINT feed_tags_name_key TO tags_name_key;

-- 5. Rename indexes
ALTER INDEX idx_feed_tags_created_at RENAME TO idx_tags_created_at;
ALTER INDEX idx_feed_tags_name RENAME TO idx_tags_name;

-- 6. Drop the existing article_tags table (it has wrong structure)
DROP TABLE IF EXISTS article_tags;

-- 7. Create the correct article_tags table
CREATE TABLE article_tags (
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    tag_id     INT  NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (article_id, tag_id)
);

ALTER TABLE article_tags
    OWNER TO alt_db_user;

CREATE INDEX idx_article_tags_tag_id
    ON article_tags (tag_id);

CREATE INDEX idx_article_tags_created_at
    ON article_tags (created_at);
