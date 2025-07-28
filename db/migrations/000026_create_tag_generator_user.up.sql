-- Create tag_generator_user if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'tag_generator_user') THEN
        CREATE USER tag_generator_user WITH PASSWORD 'tag_generator_password';
    END IF;
END
$$;