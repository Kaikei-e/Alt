-- Fix: favorite_feeds composite primary key (user_id, feed_id)
--
-- Migration 20250812001401 intended to create this PK, but it failed
-- silently when duplicate (user_id, feed_id) rows existed.
-- The migration runner recorded it as applied, leaving the table
-- without any primary key.
--
-- This migration:
--   1. Removes duplicate rows (keeps earliest created_at per pair)
--   2. Adds the composite PK if it does not already exist

-- Step 1: Deduplicate — keep only the row with the earliest created_at
-- for each (user_id, feed_id) pair
DELETE FROM favorite_feeds a
USING favorite_feeds b
WHERE a.ctid < b.ctid
  AND a.user_id = b.user_id
  AND a.feed_id = b.feed_id;

-- Step 2: Add composite primary key if not already present
-- (DO block required because ADD PRIMARY KEY has no IF NOT EXISTS)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'favorite_feeds'::regclass
          AND contype = 'p'
    ) THEN
        ALTER TABLE favorite_feeds ADD PRIMARY KEY (user_id, feed_id);
    END IF;
END
$$;
