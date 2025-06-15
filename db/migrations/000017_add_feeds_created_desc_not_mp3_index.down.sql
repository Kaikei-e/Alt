-- Remove the partial index for non-MP3 feeds
DROP INDEX IF EXISTS idx_feeds_created_desc_not_mp3;