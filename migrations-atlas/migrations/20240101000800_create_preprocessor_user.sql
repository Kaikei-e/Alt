-- Migration: create preprocessor user
-- Created: 2025-08-12 00:19:20
-- Atlas Version: v0.35
-- Source: 000008_create_preprocessor_user.up.sql

-- Create pre_processor_user if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'pre_processor_user') THEN
        CREATE USER pre_processor_user WITH PASSWORD 'pre_processor_password';
    END IF;
END
$$;
