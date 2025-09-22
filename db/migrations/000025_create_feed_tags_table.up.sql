-- Create the initial feed_tags catalog table (later renamed to tags)
CREATE TABLE feed_tags (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT feed_tags_name_key UNIQUE (name)
);

ALTER TABLE feed_tags
    OWNER TO alt_db_user;

CREATE INDEX idx_feed_tags_created_at
    ON feed_tags (created_at);

-- Maintain a dedicated index on name to match later rename expectations
CREATE INDEX idx_feed_tags_name
    ON feed_tags (name);
