-- Migration: add primary key constraint on article_tags (article_id, feed_tag_id)
-- This enables ON CONFLICT (article_id, feed_tag_id) DO NOTHING in tag-generator
-- Issue: tag-generator fails with "there is no unique or exclusion constraint matching the ON CONFLICT specification"

-- Step 1: Remove duplicate rows, keeping only the oldest one (by ctid which represents physical order)
DELETE FROM article_tags a
USING (
    SELECT article_id, feed_tag_id, MIN(ctid) as min_ctid
    FROM article_tags
    GROUP BY article_id, feed_tag_id
    HAVING COUNT(*) > 1
) b
WHERE a.article_id = b.article_id
  AND a.feed_tag_id = b.feed_tag_id
  AND a.ctid <> b.min_ctid;

-- Step 2: Add primary key constraint
ALTER TABLE article_tags
    ADD PRIMARY KEY (article_id, feed_tag_id);
