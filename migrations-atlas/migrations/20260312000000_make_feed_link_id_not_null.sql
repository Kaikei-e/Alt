-- Make feeds.feed_link_id NOT NULL after backfilling all orphan feeds.
-- All feeds now have a valid feed_link_id pointing to feed_links.
ALTER TABLE feeds ALTER COLUMN feed_link_id SET NOT NULL;
