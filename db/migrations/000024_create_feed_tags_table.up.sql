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