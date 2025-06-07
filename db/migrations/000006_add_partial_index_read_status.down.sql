-- Remove partial index for read feeds
DROP INDEX IF EXISTS idx_read_status_feed_id_read_true; 