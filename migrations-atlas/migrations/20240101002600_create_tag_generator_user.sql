-- Migration: create tag generator user
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000026_create_tag_generator_user.up.sql

-- Create tag_generator_user if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'tag_generator_user') THEN
        CREATE USER tag_generator_user WITH PASSWORD 'tag_generator_password';
    END IF;
END
$$;
