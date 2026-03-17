-- Drop tables migrated to pre-processor-db (ADR-000246 Phase 5)
-- These tables have been fully migrated to pre-processor-db and all services
-- now use pre-processor-db or Backend API for access. No service queries these
-- tables from alt-db anymore.

-- Drop in dependency order (inoreader_articles has FK to inoreader_subscriptions)
DROP TABLE IF EXISTS inoreader_articles;
DROP TABLE IF EXISTS inoreader_subscriptions;
DROP TABLE IF EXISTS sync_state;
DROP TABLE IF EXISTS api_usage_tracking;
