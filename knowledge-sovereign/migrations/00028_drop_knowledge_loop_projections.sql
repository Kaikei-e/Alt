-- atlas:nolint destructive
--
-- Retire the Knowledge Loop read models. The Loop has been superseded by the
-- Knowledge Trail (footprints + typed branches). These tables are DISPOSABLE
-- projections and idempotency barriers derived from the append-only event log —
-- dropping them removes no source of truth.
--
-- INVARIANT PRESERVED: knowledge_events is untouched. Every loop.* event ever
-- appended stays in the log forever; if the Loop ever needed to be reprojected
-- for forensics, the events remain. Only the rebuildable read side is removed.
--
-- Drop order: views (depend on entries) → tables (CASCADE clears indexes, RLS
-- policies, FKs, and any remaining dependent objects) → loop-only enum types
-- (only droppable once no column references them).

DROP VIEW IF EXISTS dangling_supersede_refs;
DROP VIEW IF EXISTS knowledge_loop_entries_public;

DROP TABLE IF EXISTS knowledge_loop_evidence CASCADE;
DROP TABLE IF EXISTS knowledge_loop_macro_state CASCADE;
DROP TABLE IF EXISTS knowledge_loop_entry_session_state CASCADE;
DROP TABLE IF EXISTS knowledge_loop_transition_dedupes CASCADE;
DROP TABLE IF EXISTS knowledge_loop_surfaces CASCADE;
DROP TABLE IF EXISTS knowledge_loop_session_state CASCADE;
DROP TABLE IF EXISTS knowledge_loop_entries CASCADE;

DROP TYPE IF EXISTS knowledge_loop_cognitive_load_hint;
DROP TYPE IF EXISTS loop_service_quality;
DROP TYPE IF EXISTS loop_priority;
DROP TYPE IF EXISTS why_kind;
DROP TYPE IF EXISTS dismiss_state;
DROP TYPE IF EXISTS surface_bucket;
DROP TYPE IF EXISTS loop_stage;
