-- Create pre_processor_user if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'pre_processor_user') THEN
        CREATE USER pre_processor_user WITH PASSWORD 'pre_processor_password';
    END IF;
END
$$;