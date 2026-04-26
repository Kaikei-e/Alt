-- Add Surface Planner v2 column + score inputs to knowledge_loop_entries.
-- Extend why_kind enum with the v3 mapping values used by the v2 planner's
-- new evidence-driven enrichment.
--
-- Reproject-safe / additive contract:
--   * surface_planner_version defaults to 1 so existing rows still resolve
--     to v1 placement; the projector switches a row to 2 only when it has
--     re-derived placement from versioned evidence.
--   * surface_score_inputs holds raw, time-bound, immutable evidence
--     (topic_overlap_count, tag_overlap_count, augur_link_id, version_drift
--     count, freshness_at, recap_topic_snapshot_id). decay/score values are
--     NEVER stored here — they are computed at render time so a reproject
--     yields the same row regardless of when it runs.
--   * The new enum variants are added in dependency order (one ADD VALUE
--     per statement; PostgreSQL forbids ALTER TYPE ... ADD VALUE in a multi-
--     statement transaction with the type's usage).
--
-- Reproject + bump procedure: docs/runbooks/knowledge-loop-reproject.md.

ALTER TABLE knowledge_loop_entries
  ADD COLUMN surface_planner_version SMALLINT NOT NULL DEFAULT 1,
  ADD COLUMN surface_score_inputs JSONB,
  ADD CONSTRAINT kle_surface_planner_version_known
    CHECK (surface_planner_version IN (1, 2));

COMMENT ON COLUMN knowledge_loop_entries.surface_planner_version IS
  'Generation of the planner that placed this entry into surface_bucket. 1 = event-type fixed mapping; 2 = knowledge-state-based decideBucketV2. Bumping requires a full reproject (runbooks/knowledge-loop-reproject.md).';

COMMENT ON COLUMN knowledge_loop_entries.surface_score_inputs IS
  'Reproject-safe evidence used by surface_planner_version=2: topic_overlap_count, tag_overlap_count, augur_link_id, version_drift_count, freshness_at, recap_topic_snapshot_id. Decay/score values are NEVER stored here.';

-- Extend why_kind enum with v3 mappings. ALTER TYPE ... ADD VALUE must run
-- outside a transaction block; Atlas handles this when each statement stands
-- alone in the migration body.
ALTER TYPE why_kind ADD VALUE IF NOT EXISTS 'topic_affinity_why';
ALTER TYPE why_kind ADD VALUE IF NOT EXISTS 'tag_trending_why';
ALTER TYPE why_kind ADD VALUE IF NOT EXISTS 'unfinished_continue_why';
