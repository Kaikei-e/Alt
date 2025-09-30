-- Migration: create read status table with multi-tenant support
-- Created: 2025-08-12 00:19:20
-- Modified: 2025-09-30 (Added user_id for multi-tenant support)
-- Atlas Version: v0.35
-- Source: 000004_create_read_status_table.up.sql

CREATE TABLE IF NOT EXISTS read_status (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    feed_id UUID NOT NULL,
    user_id UUID NOT NULL,
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    read_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Composite unique constraint: one read status per user per feed
    CONSTRAINT uq_read_status_feed_user UNIQUE (feed_id, user_id),

    CONSTRAINT fk_read_status_feed_id
        FOREIGN KEY (feed_id)
        REFERENCES feeds(id)
        ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_read_status_feed_id ON read_status (feed_id);
CREATE INDEX IF NOT EXISTS idx_read_status_user_id ON read_status (user_id);
CREATE INDEX IF NOT EXISTS idx_read_status_is_read ON read_status (is_read);
CREATE INDEX IF NOT EXISTS idx_read_status_created_at ON read_status (created_at);
CREATE INDEX IF NOT EXISTS idx_read_status_user_feed_read ON read_status (user_id, feed_id, is_read); 
