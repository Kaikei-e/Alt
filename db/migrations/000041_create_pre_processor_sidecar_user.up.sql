-- Create user for pre-processor-sidecar service
DO $$
BEGIN
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'pre_processor_sidecar_user') THEN
      CREATE USER pre_processor_sidecar_user WITH LOGIN PASSWORD 'your_password_here';
   END IF;
END $$;

-- Add comments for documentation
COMMENT ON ROLE pre_processor_sidecar_user IS 'Database user for pre-processor-sidecar CronJob service - Inoreader API integration';