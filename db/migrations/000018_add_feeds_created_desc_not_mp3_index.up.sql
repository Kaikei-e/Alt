-- Add partial index for feeds excluding MP3 files, ordered by creation date descending
-- Optimizes queries for non-audio content chronologically
CREATE INDEX idx_feeds_created_desc_not_mp3
  ON feeds (created_at DESC, link)
  WHERE link NOT LIKE '%.mp3';