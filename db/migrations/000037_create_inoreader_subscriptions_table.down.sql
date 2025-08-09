-- Drop inoreader_subscriptions table and related indexes
DROP INDEX IF EXISTS idx_inoreader_subscriptions_synced_at;
DROP INDEX IF EXISTS idx_inoreader_subscriptions_feed_url;
DROP INDEX IF EXISTS idx_inoreader_subscriptions_inoreader_id;
DROP TABLE IF EXISTS inoreader_subscriptions;