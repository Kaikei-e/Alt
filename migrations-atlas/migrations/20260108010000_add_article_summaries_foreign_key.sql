-- Migration: Add foreign key constraint to article_summaries
-- Created: 2026-01-08
-- Pattern: Safe migration with pre-validation (ADR-000059)
--
-- This migration adds a foreign key constraint from article_summaries.article_id
-- to articles.id with ON DELETE CASCADE to prevent orphaned records.

-- Step 1: Pre-validation - Check for orphaned records
DO $$
DECLARE
    orphaned_count INTEGER;
    total_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO orphaned_count
    FROM article_summaries s
    WHERE NOT EXISTS (SELECT 1 FROM articles a WHERE a.id = s.article_id);

    SELECT COUNT(*) INTO total_count FROM article_summaries;

    RAISE NOTICE 'article_summaries total: %, orphaned: %', total_count, orphaned_count;

    -- If there are orphaned records, delete them (they have no valid article)
    IF orphaned_count > 0 THEN
        RAISE NOTICE 'Deleting % orphaned article_summaries records', orphaned_count;
        DELETE FROM article_summaries s
        WHERE NOT EXISTS (SELECT 1 FROM articles a WHERE a.id = s.article_id);
    END IF;
END $$;

-- Step 2: Add foreign key constraint with ON DELETE CASCADE
-- This ensures summaries are automatically deleted when their article is deleted
ALTER TABLE article_summaries
ADD CONSTRAINT fk_article_summaries_article_id
FOREIGN KEY (article_id) REFERENCES articles(id) ON DELETE CASCADE;

-- Step 3: Add index on article_id for foreign key performance (if not exists)
-- Note: The unique index idx_article_summaries_article_user already covers this
