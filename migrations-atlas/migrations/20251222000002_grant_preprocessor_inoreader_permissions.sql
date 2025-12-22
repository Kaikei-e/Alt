-- Migration: grant permissions to pre_processor_user
-- Created: 2025-12-23 00:00:00
-- Atlas Version: v0.35

-- Grant permissions to pre_processor_user for Inoreader integration tables

-- Grant permissions on inoreader_subscriptions table
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE inoreader_subscriptions TO pre_processor_user;

-- Grant permissions on inoreader_articles table
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE inoreader_articles TO pre_processor_user;

-- Grant permissions on sync_state table
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE sync_state TO pre_processor_user;

-- Grant permissions on api_usage_tracking table
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE api_usage_tracking TO pre_processor_user;
