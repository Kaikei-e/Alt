-- Add feed_link_id to feeds table to track which RSS source each feed came from
ALTER TABLE feeds ADD COLUMN feed_link_id UUID;
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_feeds_feed_link_id'
          AND conrelid = 'feeds'::regclass
    ) THEN
        ALTER TABLE feeds ADD CONSTRAINT fk_feeds_feed_link_id
            FOREIGN KEY (feed_link_id) REFERENCES feed_links(id) ON DELETE SET NULL;
    END IF;
END;
$$;
CREATE INDEX IF NOT EXISTS idx_feeds_feed_link_id ON feeds(feed_link_id);
CREATE INDEX IF NOT EXISTS idx_feeds_feed_link_created ON feeds(feed_link_id, created_at DESC);
