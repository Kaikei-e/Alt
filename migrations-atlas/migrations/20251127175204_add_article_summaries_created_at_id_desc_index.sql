-- Add composite index for efficient keyset pagination on article_summaries
-- This index optimizes the query pattern: WHERE (created_at, id) < ($1, $2) ORDER BY created_at DESC, id DESC
-- Used by GetArticlesWithSummaries for quality checking batch processing

CREATE INDEX IF NOT EXISTS idx_article_summaries_created_at_id_desc
ON article_summaries (created_at DESC, id DESC);

-- Add comment to document the purpose
COMMENT ON INDEX idx_article_summaries_created_at_id_desc IS
'Composite index for efficient keyset pagination in quality checking. Optimizes queries filtering and sorting by (created_at, id) for batch processing of article summaries.';

