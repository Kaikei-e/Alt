-- Migration: create search indexer user
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000029_create_search_indexer_user.up.sql

-- Create search_indexer_user if it doesn't exist  
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'search_indexer_user') THEN
        CREATE USER search_indexer_user WITH PASSWORD 'search_indexer_password';
    END IF;
END
$$;
