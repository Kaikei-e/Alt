-- Drop api_usage_tracking table and related constraints/indexes
DROP INDEX IF EXISTS idx_api_usage_tracking_last_reset;
DROP INDEX IF EXISTS idx_api_usage_tracking_date;
ALTER TABLE IF EXISTS api_usage_tracking DROP CONSTRAINT IF EXISTS uq_api_usage_tracking_date;
DROP TABLE IF EXISTS api_usage_tracking;