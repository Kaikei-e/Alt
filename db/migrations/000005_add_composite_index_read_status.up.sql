-- Add composite index for efficient LEFT JOIN and NOT EXISTS queries
CREATE INDEX IF NOT EXISTS idx_read_status_feed_id_is_read ON read_status (feed_id, is_read); 