-- Add partial index for read feeds only to optimize NOT EXISTS queries
CREATE INDEX IF NOT EXISTS idx_read_status_feed_id_read_true 
ON read_status (feed_id) 
WHERE is_read = TRUE; 