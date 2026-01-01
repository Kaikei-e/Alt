-- Add unique constraint on articles (url, user_id) for multi-user ON CONFLICT upsert support
-- This allows the same URL to exist for different users

-- Drop the manually added constraint if it exists
ALTER TABLE articles DROP CONSTRAINT IF EXISTS articles_url_key;

-- Add multi-user unique constraint
ALTER TABLE articles ADD CONSTRAINT uq_articles_url_user UNIQUE (url, user_id);
