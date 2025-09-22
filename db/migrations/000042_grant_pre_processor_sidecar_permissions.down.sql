-- Revoke permissions from pre_processor_sidecar_user

-- Revoke schema permissions
REVOKE USAGE ON SCHEMA public FROM pre_processor_sidecar_user;
REVOKE CONNECT ON DATABASE alt FROM pre_processor_sidecar_user;

-- Revoke permissions on api_usage_tracking table
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE api_usage_tracking FROM pre_processor_sidecar_user;

-- Revoke permissions on sync_state table
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE sync_state FROM pre_processor_sidecar_user;

-- Revoke permissions on inoreader_articles table
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE inoreader_articles FROM pre_processor_sidecar_user;

-- Revoke permissions on inoreader_subscriptions table
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE inoreader_subscriptions FROM pre_processor_sidecar_user;
