-- Add nullable user_id to reports for per-user ownership and isolation.
ALTER TABLE reports ADD COLUMN user_id UUID;

-- Composite index for per-user report listing ordered by created_at DESC.
CREATE INDEX idx_reports_user_id_created_at ON reports (user_id, created_at DESC);
