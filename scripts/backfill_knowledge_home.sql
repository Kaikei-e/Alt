-- Backfill script for Knowledge Home existing data recovery
-- Run AFTER deploying code fixes for Bug 1-4.
--
-- Step 1: Backfill summary_versions from article_summaries
-- Step 2: Generate SummaryVersionCreated events in knowledge_events
-- Step 3: Reset projection checkpoint so projector re-processes all events
--
-- Usage:
--   psql -d alt -f scripts/backfill_knowledge_home.sql
-- Or via Docker:
--   docker exec -i alt-db psql -U postgres -d alt < scripts/backfill_knowledge_home.sql

BEGIN;

-- Step 1: Backfill summary_versions from article_summaries (skip duplicates)
INSERT INTO summary_versions (
    summary_version_id, article_id, user_id, generated_at,
    model, prompt_version, input_hash, quality_score, summary_text
)
SELECT
    gen_random_uuid(),
    a.article_id,
    COALESCE(a.user_id, '00000000-0000-0000-0000-000000000000'::uuid),
    COALESCE(a.updated_at, a.created_at, now()),
    'backfill',
    'v0',
    md5(a.summary),
    NULL,
    a.summary
FROM article_summaries a
LEFT JOIN summary_versions sv ON sv.article_id = a.article_id AND sv.superseded_by IS NULL
WHERE a.summary IS NOT NULL
  AND a.summary != ''
  AND sv.summary_version_id IS NULL;

-- Step 2: Generate SummaryVersionCreated events for newly backfilled versions
INSERT INTO knowledge_events (
    event_id, occurred_at, tenant_id, user_id,
    actor_type, actor_id, event_type,
    aggregate_type, aggregate_id,
    dedupe_key, payload
)
SELECT
    gen_random_uuid(),
    sv.generated_at,
    sv.user_id,
    sv.user_id,
    'service',
    'backfill',
    'SummaryVersionCreated',
    'article',
    sv.article_id::text,
    'SummaryVersionCreated:' || sv.summary_version_id::text,
    jsonb_build_object(
        'summary_version_id', sv.summary_version_id::text,
        'article_id', sv.article_id::text,
        'model', sv.model,
        'prompt_version', sv.prompt_version
    )
FROM summary_versions sv
LEFT JOIN knowledge_events ke
    ON ke.dedupe_key = 'SummaryVersionCreated:' || sv.summary_version_id::text
WHERE sv.model = 'backfill'
  AND ke.event_id IS NULL;

-- Step 3: Reset projection checkpoint to re-process all events
-- The projector will pick up from seq 0 and re-project everything
UPDATE knowledge_projection_checkpoints
SET last_event_seq = 0, updated_at = now()
WHERE projector_name = 'knowledge-home-projector';

-- If no checkpoint row exists yet, insert one at 0
INSERT INTO knowledge_projection_checkpoints (projector_name, last_event_seq, updated_at)
VALUES ('knowledge-home-projector', 0, now())
ON CONFLICT (projector_name) DO NOTHING;

COMMIT;

-- Report results
SELECT 'summary_versions backfilled' AS step, count(*) AS cnt
FROM summary_versions WHERE model = 'backfill'
UNION ALL
SELECT 'SummaryVersionCreated events' AS step, count(*) AS cnt
FROM knowledge_events WHERE event_type = 'SummaryVersionCreated' AND actor_id = 'backfill'
UNION ALL
SELECT 'projection checkpoint' AS step, last_event_seq AS cnt
FROM knowledge_projection_checkpoints WHERE projector_name = 'knowledge-home-projector';
