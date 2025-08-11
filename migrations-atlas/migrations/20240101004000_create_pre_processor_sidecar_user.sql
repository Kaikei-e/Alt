-- Migration: create pre processor sidecar user
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000041_create_pre_processor_sidecar_user.up.sql

-- Create user for pre-processor-sidecar service
DO $$
BEGIN
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'pre_processor_sidecar_user') THEN
      CREATE USER pre_processor_sidecar_user WITH LOGIN PASSWORD 'your_password_here';
   END IF;
END $$;

-- Add comments for documentation
COMMENT ON ROLE pre_processor_sidecar_user IS 'Database user for pre-processor-sidecar CronJob service - Inoreader API integration';
