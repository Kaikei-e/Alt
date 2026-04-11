-- Feed Read Performance: EXPLAIN ANALYZE
-- Run manually via: psql -h localhost -p 5432 -U <user> -d <dbname> -f scripts/explain_feed_queries.sql
-- Replace <test_user_id> with an actual user UUID before running.

\set test_user_id '00000000-0000-0000-0000-000000000000'

\echo '=== 1. GetUnreadFeeds (FetchUnreadFeedsListCursor) ==='
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at,
       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
       f.og_image_url
FROM feeds f
WHERE NOT EXISTS (
    SELECT 1
    FROM read_status rs
    WHERE rs.feed_id = f.id
    AND rs.user_id = :'test_user_id'
    AND rs.is_read = TRUE
)
AND (f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = :'test_user_id') OR f.feed_link_id IS NULL)
ORDER BY f.created_at DESC, f.id DESC
LIMIT 21;

\echo ''
\echo '=== 2. GetAllFeeds (FetchAllFeedsListCursor) ==='
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at,
       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
       COALESCE(rs.is_read, FALSE) AS is_read,
       f.og_image_url
FROM feeds f
LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.user_id = :'test_user_id'
WHERE (f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = :'test_user_id') OR f.feed_link_id IS NULL)
ORDER BY f.created_at DESC, f.id DESC
LIMIT 21;

\echo ''
\echo '=== 3. GetReadFeeds (FetchReadFeedsListCursor) ==='
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at
FROM feeds f
INNER JOIN read_status rs ON rs.feed_id = f.id
WHERE rs.is_read = TRUE
AND rs.user_id = :'test_user_id'
AND (f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = :'test_user_id') OR f.feed_link_id IS NULL)
ORDER BY rs.read_at DESC, f.id DESC
LIMIT 21;

\echo ''
\echo '=== 4. FetchArticlesWithCursor ==='
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT
    a.id,
    a.title,
    a.url,
    a.content,
    a.created_at as published_at,
    a.created_at,
    COALESCE(tags.tag_names, '{}') as tags
FROM (
    SELECT id, title, url, content, created_at
    FROM articles
    WHERE user_id = :'test_user_id' AND deleted_at IS NULL
    ORDER BY created_at DESC, id DESC
    LIMIT 21
) a
LEFT JOIN LATERAL (
    SELECT ARRAY_AGG(ft.tag_name) as tag_names
    FROM article_tags at
    JOIN feed_tags ft ON at.feed_tag_id = ft.id
    WHERE at.article_id = a.id
) tags ON TRUE
ORDER BY a.created_at DESC, a.id DESC;

\echo ''
\echo '=== 5. pg_stat_statements: Top 20 by total_exec_time ==='
SELECT
    substring(query, 1, 80) AS query_preview,
    calls,
    round(total_exec_time::numeric, 2) AS total_ms,
    round(mean_exec_time::numeric, 2) AS mean_ms,
    round(max_exec_time::numeric, 2) AS max_ms,
    rows,
    shared_blks_hit,
    shared_blks_read
FROM pg_stat_statements
ORDER BY total_exec_time DESC
LIMIT 20;
