-- Drop sovereign-owned tables from alt-db.
-- These tables now live exclusively in knowledge-sovereign-db.
-- See: docs/plan/knowledge-sovereign-full-separation.md

DROP TABLE IF EXISTS knowledge_current_lens;
DROP TABLE IF EXISTS knowledge_lens_versions;
DROP TABLE IF EXISTS knowledge_lenses;
DROP TABLE IF EXISTS knowledge_projection_audits;
DROP TABLE IF EXISTS knowledge_reproject_runs;
DROP TABLE IF EXISTS knowledge_backfill_jobs;
DROP TABLE IF EXISTS knowledge_projection_checkpoints;
DROP TABLE IF EXISTS knowledge_projection_versions;
DROP TABLE IF EXISTS recall_signals;
DROP TABLE IF EXISTS recall_candidate_view;
DROP TABLE IF EXISTS today_digest_view;
DROP TABLE IF EXISTS knowledge_home_items;
DROP TABLE IF EXISTS knowledge_user_events;
DROP TABLE IF EXISTS knowledge_events;
