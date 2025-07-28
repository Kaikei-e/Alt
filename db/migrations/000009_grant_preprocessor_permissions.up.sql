-- Grant specific table permissions to the preprocessor user
-- This runs after all tables have been created by previous migrations
-- Note: This assumes the user is named 'pre_processor_user' based on the environment variable

-- Grant permissions on feeds table (read access for getting RSS URLs)
GRANT SELECT ON TABLE feeds TO pre_processor_user;

-- Grant permissions on articles table (read/write access for storing processed articles)
GRANT SELECT, INSERT, UPDATE ON TABLE articles TO pre_processor_user;

-- Ensure the user has access to sequences used by these tables
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO pre_processor_user;