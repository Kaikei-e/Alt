-- Migration: add unique constraint feeds link
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000003_add_unique_constraint_feeds_link.up.sql

DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'unique_feeds_link') THEN
        ALTER TABLE feeds ADD CONSTRAINT unique_feeds_link UNIQUE (link);
    END IF;
END
$$; 
