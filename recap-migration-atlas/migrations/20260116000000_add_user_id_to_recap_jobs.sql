-- Add user_id and trigger_source to recap_jobs for user-specific job tracking
ALTER TABLE recap_jobs
ADD COLUMN IF NOT EXISTS user_id UUID,
ADD COLUMN IF NOT EXISTS trigger_source TEXT NOT NULL DEFAULT 'system';

-- Add index for user_id queries
CREATE INDEX IF NOT EXISTS idx_recap_jobs_user_id ON recap_jobs(user_id);

-- Add feed_id and original_user_id to recap_job_articles for user article tracking
ALTER TABLE recap_job_articles
ADD COLUMN IF NOT EXISTS feed_id UUID,
ADD COLUMN IF NOT EXISTS original_user_id UUID;

-- Add indexes for user article queries
CREATE INDEX IF NOT EXISTS idx_recap_job_articles_user_id ON recap_job_articles(original_user_id);
CREATE INDEX IF NOT EXISTS idx_recap_job_articles_feed_id ON recap_job_articles(feed_id);
