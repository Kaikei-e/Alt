-- Migration: create alt_appuser and grant permissions
-- Created: 2025-11-28
-- Atlas Version: v0.35

-- Create alt_appuser if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'alt_appuser') THEN
        CREATE USER alt_appuser WITH PASSWORD 'alt_appuser_password';
    END IF;
END
$$;

-- Grant permissions on schema public
GRANT USAGE ON SCHEMA public TO alt_appuser;

-- Grant permissions on all tables in schema public
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO alt_appuser;

-- Grant permissions on all sequences in schema public
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO alt_appuser;

-- Ensure future tables are accessible
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO alt_appuser;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO alt_appuser;
