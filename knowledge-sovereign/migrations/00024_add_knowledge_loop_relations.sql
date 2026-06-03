-- ADR-000937: relation-set as the primary axis of the Knowledge Loop.
--
-- relations is JSONB opaque, populated by the projector from the same
-- SurfaceScoreInputs decideBucketV2 reads — but kept as a first-class set
-- instead of being collapsed into a single surface_bucket. The Orient surface
-- renders the set directly; surface_bucket is demoted to a lens derived from
-- it. Reproject-safe: extractRelations is a pure function of the inputs, so a
-- full reproject reproduces identical relations. Bumping the extraction logic
-- requires a full reproject (runbooks/knowledge-loop-reproject.md).

ALTER TABLE knowledge_loop_entries
  ADD COLUMN relations JSONB;

COMMENT ON COLUMN knowledge_loop_entries.relations IS
  'ADR-000937 relation-set: [{kind,target_ref,magnitude,state,why_text}]. First-class evidence kept un-collapsed so the Orient surface can render relations directly and close the loop via a relation State transition on return. surface_bucket is demoted to a lens over this set.';

-- Refresh the read-facing view to expose relations. Mirrors the column list
-- from 00022 plus relations appended at the end.
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
  render_depth_hint,
  loop_priority,
  surface_planner_version,
  surface_score_inputs,
  visibility_state,
  completion_state,
  why_counter_evidence_refs,
  why_confidence_ladder,
  why_what_would_change_my_mind,
  review_reason,
  relations
FROM knowledge_loop_entries;
