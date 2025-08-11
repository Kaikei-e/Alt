-- Migration: add partial index read status
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000006_add_partial_index_read_status.up.sql

-- Add partial index for read feeds only to optimize NOT EXISTS queries
CREATE INDEX IF NOT EXISTS idx_read_status_feed_id_read_true 
ON read_status (feed_id) 
WHERE is_read = TRUE; 
