-- Split Knowledge Loop read visibility from legacy dismiss_state.
--
-- dismiss_state is retained as a compatibility field for old clients and
-- existing projectors, but GetKnowledgeLoopEntries now filters on
-- visibility_state. This lets HomeItemOpened entries remain visible in
-- Continue and HomeItemDismissed entries remain visible in Review while
-- explicit Loop defer/archive/mark_reviewed actions still hide rows.

CREATE TYPE loop_visibility_state AS ENUM (
  'visible',
  'hidden',
  'snoozed'
);

CREATE TYPE loop_completion_state AS ENUM (
  'open',
  'completed',
  'dismissed'
);

ALTER TABLE knowledge_loop_entries
  ADD COLUMN visibility_state loop_visibility_state NOT NULL DEFAULT 'visible',
  ADD COLUMN completion_state loop_completion_state NOT NULL DEFAULT 'open';

UPDATE knowledge_loop_entries
SET
  visibility_state = CASE
    WHEN dismiss_state = 'active' THEN 'visible'::loop_visibility_state
    WHEN dismiss_state = 'dismissed' AND surface_bucket = 'review' THEN 'visible'::loop_visibility_state
    WHEN dismiss_state = 'completed' AND surface_bucket = 'continue' THEN 'visible'::loop_visibility_state
    WHEN dismiss_state = 'deferred' THEN 'snoozed'::loop_visibility_state
    ELSE 'hidden'::loop_visibility_state
  END,
  completion_state = CASE
    WHEN dismiss_state = 'completed' THEN 'completed'::loop_completion_state
    WHEN dismiss_state = 'dismissed' THEN 'dismissed'::loop_completion_state
    ELSE 'open'::loop_completion_state
  END;

CREATE INDEX idx_kle_visible_surface_rank
  ON knowledge_loop_entries (
    user_id,
    lens_mode_id,
    surface_bucket,
    loop_priority,
    render_depth_hint DESC,
    freshness_at DESC,
    projection_seq_hiwater DESC
  )
  WHERE visibility_state = 'visible';

CREATE OR REPLACE VIEW knowledge_loop_entries_public AS
SELECT
  user_id,
  tenant_id,
  lens_mode_id,
  entry_key,
  source_item_key,
  proposed_stage,
  surface_bucket,
  projection_revision,
  projection_seq_hiwater,
  source_event_seq,
  freshness_at,
  source_observed_at,
  artifact_summary_version_id,
  artifact_tag_set_version_id,
  artifact_lens_version_id,
  why_kind,
  why_text,
  why_confidence,
  why_evidence_ref_ids,
  why_evidence_refs,
  change_summary,
  continue_context,
  decision_options,
  act_targets,
  superseded_by_entry_key,
  dismiss_state,
  visibility_state,
  completion_state,
  render_depth_hint,
  loop_priority,
  surface_planner_version,
  surface_score_inputs
FROM knowledge_loop_entries;

COMMENT ON COLUMN knowledge_loop_entries.visibility_state IS
  'Read-path visibility lifecycle. GetKnowledgeLoopEntries filters on this field; dismiss_state is compatibility metadata.';
COMMENT ON COLUMN knowledge_loop_entries.completion_state IS
  'Semantic completion lifecycle independent of visibility.';
COMMENT ON VIEW knowledge_loop_entries_public IS
  'Read-handler-facing view that intentionally excludes projected_at. All Get/Stream handlers MUST read through this view; direct SELECT on knowledge_loop_entries is projector-only.';
