CREATE TABLE IF NOT EXISTS feed_links (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    url TEXT NOT NULL UNIQUE,
    CONSTRAINT idx_feed_links_id_url UNIQUE (id, url)
);