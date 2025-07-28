-- Create the favorite_feeds table
CREATE TABLE IF NOT EXISTS favorite_feeds (
    feed_id    UUID PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_favorite_feeds_feed_id
        FOREIGN KEY (feed_id)
        REFERENCES feeds(id)
        ON DELETE CASCADE
);

-- Index for ordering favorites by creation date
CREATE INDEX IF NOT EXISTS idx_favorite_feeds_created_at ON favorite_feeds (created_at DESC);