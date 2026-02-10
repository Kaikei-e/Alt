-- Create user_feed_subscriptions table to track which feed sources each user subscribes to
CREATE TABLE user_feed_subscriptions (
    user_id UUID NOT NULL,
    feed_link_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, feed_link_id),
    CONSTRAINT fk_ufs_feed_link_id
        FOREIGN KEY (feed_link_id) REFERENCES feed_links(id) ON DELETE CASCADE
);
CREATE INDEX idx_ufs_user_id ON user_feed_subscriptions(user_id);
CREATE INDEX idx_ufs_user_created ON user_feed_subscriptions(user_id, created_at DESC);
