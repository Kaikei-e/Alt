-- Migration: add composite index read status
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000005_add_composite_index_read_status.up.sql

-- Add composite index for efficient LEFT JOIN and NOT EXISTS queries
CREATE INDEX IF NOT EXISTS idx_read_status_feed_id_is_read ON read_status (feed_id, is_read); 
