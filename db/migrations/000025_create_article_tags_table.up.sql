-- Create the article_tags junction table
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