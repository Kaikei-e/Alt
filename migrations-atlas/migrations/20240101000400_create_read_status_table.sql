-- Migration: create read status table
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000004_create_read_status_table.up.sql

CREATE TABLE IF NOT EXISTS read_status (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    feed_id UUID NOT NULL UNIQUE,
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    read_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT fk_read_status_feed_id 
        FOREIGN KEY (feed_id) 
        REFERENCES feeds(id) 
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_read_status_feed_id ON read_status (feed_id);
CREATE INDEX IF NOT EXISTS idx_read_status_is_read ON read_status (is_read);
CREATE INDEX IF NOT EXISTS idx_read_status_created_at ON read_status (created_at); 
