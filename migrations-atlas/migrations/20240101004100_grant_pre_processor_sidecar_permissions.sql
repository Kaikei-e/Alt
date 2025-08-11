-- Migration: grant pre processor sidecar permissions
-- Created: 2025-08-12 00:19:21
-- Atlas Version: v0.35
-- Source: 000042_grant_pre_processor_sidecar_permissions.up.sql

-- Grant permissions to pre_processor_sidecar_user for Inoreader integration tables

-- Grant permissions on inoreader_subscriptions table
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE inoreader_subscriptions TO pre_processor_sidecar_user;
GRANT USAGE ON SEQUENCE inoreader_subscriptions_id_seq TO pre_processor_sidecar_user;

-- Grant permissions on inoreader_articles table  
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE inoreader_articles TO pre_processor_sidecar_user;
GRANT USAGE ON SEQUENCE inoreader_articles_id_seq TO pre_processor_sidecar_user;

-- Grant permissions on sync_state table
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE sync_state TO pre_processor_sidecar_user;
GRANT USAGE ON SEQUENCE sync_state_id_seq TO pre_processor_sidecar_user;

-- Grant permissions on api_usage_tracking table
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE api_usage_tracking TO pre_processor_sidecar_user;
GRANT USAGE ON SEQUENCE api_usage_tracking_id_seq TO pre_processor_sidecar_user;

-- Grant CONNECT permission on database
GRANT CONNECT ON DATABASE alt TO pre_processor_sidecar_user;

-- Grant USAGE on public schema
GRANT USAGE ON SCHEMA public TO pre_processor_sidecar_user;
