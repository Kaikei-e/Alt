-- Migration: add create_at to sync_state table
-- Created: 2025-12-22
-- Reason: Fix missing column causing sync failures

ALTER TABLE sync_state ADD COLUMN IF NOT EXISTS created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW();
