CREATE INDEX IF NOT EXISTS idx_feeds_desc_not_mp3_cover
  ON feeds (created_at DESC, id DESC)
  INCLUDE (link)
  WHERE link NOT LIKE '%.mp3';