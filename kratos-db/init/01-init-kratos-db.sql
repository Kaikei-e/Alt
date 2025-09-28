-- Kratos Database Initialization Script
-- This script initializes the Kratos database with the necessary permissions

-- Create the kratos database if it doesn't exist
-- Note: The database is already created by the POSTGRES_DB environment variable
-- This script is for any additional setup if needed

-- Grant necessary permissions to the kratos user
GRANT ALL PRIVILEGES ON DATABASE kratos TO kratos_user;

-- Ensure the kratos_user has the necessary permissions
ALTER USER kratos_user CREATEDB;
