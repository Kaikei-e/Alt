CREATE TABLE IF NOT EXISTS feeds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    link TEXT NOT NULL,
    pub_date TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_feeds_created_at ON feeds (created_at);
CREATE INDEX IF NOT EXISTS idx_feeds_id_link ON feeds (id, link);