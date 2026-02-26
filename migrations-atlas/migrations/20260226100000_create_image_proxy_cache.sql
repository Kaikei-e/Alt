-- Create image_proxy_cache table for OGP image proxy caching.
-- Stores WebP-compressed images fetched from external sources.
-- Separated from article_heads for concern separation (binary data vs metadata).

CREATE TABLE IF NOT EXISTS image_proxy_cache (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    url_hash TEXT NOT NULL UNIQUE,
    original_url TEXT NOT NULL,
    image_data BYTEA NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'image/webp',
    width INT NOT NULL,
    height INT NOT NULL,
    size_bytes INT NOT NULL,
    etag TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    CONSTRAINT image_proxy_cache_size_limit CHECK (size_bytes <= 1048576)
);

CREATE INDEX idx_ipc_url_hash ON image_proxy_cache (url_hash);
CREATE INDEX idx_ipc_expires_at ON image_proxy_cache (expires_at);
