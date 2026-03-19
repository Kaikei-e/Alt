-- Backfill script for Knowledge Home existing data recovery
-- Run AFTER deploying code fixes for Bug 1-4.
--
-- Step 1: Backfill summary_versions from article_summaries
-- Step 2: Generate SummaryVersionCreated events in knowledge_events
-- Step 3: Backfill tag_set_versions from article_tag_normalized
-- Step 4: Generate TagSetVersionCreated events in knowledge_events
-- Step 5: Reset projection checkpoint so projector re-processes all events
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
    a.user_id,
    COALESCE(a.created_at, now()),
    'backfill',
    'v0',
    md5(a.summary_japanese),
    NULL,
    a.summary_japanese
FROM article_summaries a
LEFT JOIN summary_versions sv ON sv.article_id = a.article_id AND sv.superseded_by IS NULL
WHERE a.summary_japanese IS NOT NULL
  AND a.summary_japanese != ''
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

-- Step 3: Backfill tag_set_versions from article_tags + feed_tags
-- Build a tag snapshot per article (aggregating all tags via feed_tags join)
INSERT INTO tag_set_versions (
    tag_set_version_id, article_id, user_id, generated_at,
    generator, input_hash, tags_json, superseded_by
)
SELECT
    gen_random_uuid(),
    at.article_id,
    a.user_id,
    COALESCE(max(at.created_at), now()),
    'backfill',
    md5(string_agg(ft.tag_name, ',' ORDER BY ft.tag_name)),
    jsonb_agg(
        jsonb_build_object('name', ft.tag_name, 'confidence', COALESCE(ft.confidence, 0.5))
        ORDER BY ft.tag_name
    ),
    NULL
FROM article_tags at
JOIN feed_tags ft ON ft.id = at.feed_tag_id
JOIN articles a ON a.id = at.article_id
LEFT JOIN tag_set_versions tsv ON tsv.article_id = at.article_id AND tsv.superseded_by IS NULL
WHERE tsv.tag_set_version_id IS NULL
GROUP BY at.article_id, a.user_id
HAVING count(*) > 0;

-- Step 4: Generate TagSetVersionCreated events for newly backfilled tag versions
INSERT INTO knowledge_events (
    event_id, occurred_at, tenant_id, user_id,
    actor_type, actor_id, event_type,
    aggregate_type, aggregate_id,
    dedupe_key, payload
)
SELECT
    gen_random_uuid(),
    tsv.generated_at,
    tsv.user_id,
    tsv.user_id,
    'service',
    'backfill',
    'TagSetVersionCreated',
    'article',
    tsv.article_id::text,
    'TagSetVersionCreated:' || tsv.tag_set_version_id::text,
    jsonb_build_object(
        'tag_set_version_id', tsv.tag_set_version_id::text,
        'article_id', tsv.article_id::text,
        'generator', tsv.generator
    )
FROM tag_set_versions tsv
LEFT JOIN knowledge_events ke
    ON ke.dedupe_key = 'TagSetVersionCreated:' || tsv.tag_set_version_id::text
WHERE tsv.generator = 'backfill'
  AND ke.event_id IS NULL;

-- Step 5: Reset projection checkpoint to re-process all events
-- The projector will pick up from seq 0 and re-project everything
-- Note: no atlas migration changes, so no atlas.sum update needed
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
SELECT 'tag_set_versions backfilled' AS step, count(*) AS cnt
FROM tag_set_versions WHERE generator = 'backfill'
UNION ALL
SELECT 'TagSetVersionCreated events' AS step, count(*) AS cnt
FROM knowledge_events WHERE event_type = 'TagSetVersionCreated' AND actor_id = 'backfill'
UNION ALL
SELECT 'items with tags' AS step, count(*) AS cnt
FROM knowledge_home_items WHERE tags_json IS NOT NULL AND tags_json != '[]'
UNION ALL
SELECT 'items with tag_hotspot' AS step, count(*) AS cnt
FROM knowledge_home_items WHERE why_json::text LIKE '%tag_hotspot%'
UNION ALL
SELECT 'projection checkpoint' AS step, last_event_seq AS cnt
FROM knowledge_projection_checkpoints WHERE projector_name = 'knowledge-home-projector';
