-- Reverse the tags schema fix

-- 1. Drop the corrected article_tags table
DROP TABLE IF EXISTS article_tags;

-- 2. Rename indexes back
ALTER INDEX idx_tags_name RENAME TO idx_feed_tags_name;
ALTER INDEX idx_tags_created_at RENAME TO idx_feed_tags_created_at;

-- 3. Rename constraints back
ALTER TABLE tags RENAME CONSTRAINT tags_name_key TO feed_tags_name_key;
ALTER TABLE tags RENAME CONSTRAINT tags_pkey TO feed_tags_pkey;

-- 4. Rename sequence back
ALTER SEQUENCE tags_id_seq RENAME TO feed_tags_id_seq;

-- 5. Rename table back to feed_tags
ALTER TABLE tags RENAME TO feed_tags;

-- 6. Recreate the original (incorrect) article_tags table
CREATE TABLE article_tags (
    article_id UUID     NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    feed_tag_id     INT      NOT NULL REFERENCES feed_tags(id)     ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (article_id, feed_tag_id)
);

ALTER TABLE article_tags
    OWNER TO alt_db_user;

CREATE INDEX idx_article_tags_feed_tag_id
    ON article_tags (feed_tag_id);

CREATE INDEX idx_article_tags_created_at
    ON article_tags (created_at);