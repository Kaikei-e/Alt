-- Create article_heads table for storing <head> section and OGP metadata
-- Used by Visual Preview mode to display og:image thumbnails
-- TTL: 12 hours (cleanup via scheduled job)

CREATE TABLE IF NOT EXISTS article_heads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    head_html TEXT NOT NULL,
    og_image_url TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (article_id)
);

CREATE INDEX idx_article_heads_article_id ON article_heads (article_id);
CREATE INDEX idx_article_heads_created_at ON article_heads (created_at);
