-- Migration: create sync state table
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000039_create_sync_state_table.up.sql

-- Create sync_state table for managing continuation tokens and synchronization state
CREATE TABLE IF NOT EXISTS sync_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stream_id TEXT UNIQUE NOT NULL,
    continuation_token TEXT,
    last_sync TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_sync_state_stream_id ON sync_state(stream_id);
CREATE INDEX IF NOT EXISTS idx_sync_state_last_sync ON sync_state(last_sync DESC);

-- Add comments for documentation
COMMENT ON TABLE sync_state IS 'Stores synchronization state and continuation tokens for Inoreader stream pagination';
COMMENT ON COLUMN sync_state.id IS 'Internal UUID primary key';
COMMENT ON COLUMN sync_state.stream_id IS 'Stream identifier (e.g., user/-/state/com.google/reading-list)';
COMMENT ON COLUMN sync_state.continuation_token IS 'Continuation token for pagination from Inoreader API';
COMMENT ON COLUMN sync_state.last_sync IS 'Last successful synchronization timestamp';
