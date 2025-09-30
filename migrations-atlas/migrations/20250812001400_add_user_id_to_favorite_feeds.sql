-- Add user_id to favorite_feeds table for multi-user support
-- Phase 1: Add nullable user_id column
-- PostgreSQL migration for multi-tenant favorite feeds support

-- Add user_id column (nullable initially to allow data migration)
ALTER TABLE favorite_feeds
ADD COLUMN user_id UUID;