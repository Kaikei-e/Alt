-- Grant permissions to pre_processor_sidecar_user for Inoreader integration tables

-- Grant permissions on inoreader_subscriptions table
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE inoreader_subscriptions TO pre_processor_sidecar_user;

-- Grant permissions on inoreader_articles table  
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE inoreader_articles TO pre_processor_sidecar_user;

-- Grant permissions on sync_state table
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE sync_state TO pre_processor_sidecar_user;

-- Grant permissions on api_usage_tracking table
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE api_usage_tracking TO pre_processor_sidecar_user;

-- Grant CONNECT permission on database
GRANT CONNECT ON DATABASE alt TO pre_processor_sidecar_user;

-- Grant USAGE on public schema
GRANT USAGE ON SCHEMA public TO pre_processor_sidecar_user;
