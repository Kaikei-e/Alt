-- Migration: Create feed_link_availability table for tracking feed health
-- Created: 2026-01-11 00:00:00
-- Atlas Version: v0.35

-- Fix missing PRIMARY KEY constraint on feed_links.id (required for FK reference)
-- First remove duplicate rows, keeping the first occurrence
DELETE FROM feed_links a USING feed_links b
WHERE a.ctid > b.ctid AND a.id = b.id;

-- Now add PRIMARY KEY if missing
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'feed_links'::regclass AND contype = 'p'
    ) THEN
        ALTER TABLE feed_links ADD PRIMARY KEY (id);
    END IF;
END $$;

-- Create feed_link_availability table for tracking feed health
CREATE TABLE IF NOT EXISTS feed_link_availability (
    feed_link_id UUID PRIMARY KEY REFERENCES feed_links(id) ON DELETE CASCADE,
    is_active BOOLEAN NOT NULL DEFAULT true,
    consecutive_failures INT NOT NULL DEFAULT 0,
    last_failure_at TIMESTAMP,
    last_failure_reason TEXT
);

-- Index for querying active feeds efficiently
CREATE INDEX IF NOT EXISTS idx_feed_link_availability_active
ON feed_link_availability(is_active) WHERE is_active = true;

-- Initialize availability for existing feed_links (all active by default)
INSERT INTO feed_link_availability (feed_link_id, is_active, consecutive_failures)
SELECT id, true, 0 FROM feed_links
ON CONFLICT (feed_link_id) DO NOTHING;
