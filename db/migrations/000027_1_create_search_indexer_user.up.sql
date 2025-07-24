-- Create search_indexer_user if it doesn't exist  
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'search_indexer_user') THEN
        CREATE USER search_indexer_user WITH PASSWORD 'search_indexer_password';
    END IF;
END
$$;