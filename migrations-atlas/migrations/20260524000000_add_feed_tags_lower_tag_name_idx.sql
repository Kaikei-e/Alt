-- atlas:txmode none
--
-- Add functional B-tree on lower(tag_name) so the case-insensitive prefix
-- predicate `WHERE lower(tag_name) LIKE lower($1) || '%'` runs as an index
-- range scan instead of a parallel seq scan over the 240k-row feed_tags
-- table. text_pattern_ops is required because the default opclass uses the
-- collation-aware ~<~ / ~>=~ operators that LIKE prefix optimisation does
-- not target in non-C locales (PG 17 indexes-opclass).
--
-- Production EXPLAIN ANALYZE before this index:
--   Parallel Seq Scan on feed_tags (240,520 rows scanned, filter discards
--   224,664), parallel hash join + group aggregate, Execution Time 180 ms.
-- Expected after: Index Range Scan on idx_feed_tags_tag_name_lower, single
-- digit ms execution time (Crunchy Data + Paul Ramsey case study).
--
-- IF NOT EXISTS guards the migration when re-applied against a database that
-- already carries the index (e.g. when restoring from an environment that
-- ran the index out-of-band). CONCURRENTLY (+ atlas:txmode none, since
-- Postgres forbids it inside a transaction) avoids blocking writes to the
-- 240k-row feed_tags table for the duration of the build.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_feed_tags_tag_name_lower
    ON feed_tags (lower(tag_name) text_pattern_ops);

ANALYZE feed_tags;
