-- Migration: add user_id to article_summaries with composite unique constraint
-- Created: 2026-01-08
-- Atlas Version: v0.35
--
-- Problem: article_summaries.article_id lacks unique constraint, causing
-- ON CONFLICT (article_id) to fail. Adding user_id for multi-tenant support.

-- 1. Add user_id column (nullable initially for data migration)
ALTER TABLE article_summaries
ADD COLUMN user_id UUID;

-- 2. Populate user_id from related articles
UPDATE article_summaries s
SET user_id = a.user_id
FROM articles a
WHERE s.article_id = a.id;

-- 3. Handle orphaned summaries (articles deleted but summaries remain)
-- Delete summaries where article no longer exists
DELETE FROM article_summaries
WHERE user_id IS NULL;

-- 4. Add NOT NULL constraint after data migration
ALTER TABLE article_summaries
ALTER COLUMN user_id SET NOT NULL;

-- 5. Remove duplicate summaries (keep the most recent one per article+user)
-- Using CTE with row_number to handle edge cases (same created_at)
DELETE FROM article_summaries
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY article_id, user_id
                   ORDER BY created_at DESC, id DESC
               ) as rn
        FROM article_summaries
    ) ranked
    WHERE rn > 1
);

-- 6. Create composite unique index
CREATE UNIQUE INDEX idx_article_summaries_article_user
ON article_summaries (article_id, user_id);

-- 7. Drop redundant non-unique index (now covered by unique index)
DROP INDEX IF EXISTS idx_article_summaries_article_id;

-- 8. Add index for user_id lookups
CREATE INDEX idx_article_summaries_user_id
ON article_summaries (user_id);
