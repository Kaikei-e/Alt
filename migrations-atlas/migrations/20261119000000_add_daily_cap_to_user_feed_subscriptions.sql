-- Add per-subscription daily unread cap (NULL = unlimited).
ALTER TABLE user_feed_subscriptions
    ADD COLUMN daily_cap INTEGER NULL,
    ADD CONSTRAINT chk_user_feed_subscriptions_daily_cap_positive
        CHECK (daily_cap IS NULL OR daily_cap > 0);

-- Support index-only rank counting over (feed_link_id, created_at ASC, id ASC).
CREATE INDEX IF NOT EXISTS idx_feeds_feed_link_created_id
    ON feeds (feed_link_id, created_at ASC, id ASC);
