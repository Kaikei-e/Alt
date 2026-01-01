-- Migration: create user_reading_status table for article-level read tracking
-- Created: 2026-01-01
-- Purpose: Track individual article read status per user (separate from feed-level read_status)

CREATE TABLE IF NOT EXISTS user_reading_status (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    article_id UUID NOT NULL,
    is_read BOOLEAN NOT NULL DEFAULT TRUE,
    read_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- One read status per user per article
    CONSTRAINT uq_user_reading_status UNIQUE (user_id, article_id),

    -- Foreign key to articles table
    CONSTRAINT fk_user_reading_status_article_id
        FOREIGN KEY (article_id)
        REFERENCES articles(id)
        ON DELETE CASCADE
);

-- Indexes for query performance
CREATE INDEX IF NOT EXISTS idx_user_reading_status_user_id ON user_reading_status(user_id);
CREATE INDEX IF NOT EXISTS idx_user_reading_status_article_id ON user_reading_status(article_id);
CREATE INDEX IF NOT EXISTS idx_user_reading_status_user_article ON user_reading_status(user_id, article_id);
CREATE INDEX IF NOT EXISTS idx_user_reading_status_read_at ON user_reading_status(read_at);
