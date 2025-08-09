-- Drop sync_state table and related indexes
DROP INDEX IF EXISTS idx_sync_state_last_sync;
DROP INDEX IF EXISTS idx_sync_state_stream_id;
DROP TABLE IF EXISTS sync_state;