-- Refresh the read-facing Knowledge Loop view after migration 00013 added
-- Surface Planner v2 metadata columns to knowledge_loop_entries.
--
-- projected_at stays intentionally absent. surface_score_inputs is carried
-- only across the sovereign-internal RPC boundary for diagnostics/reproject
-- verification; the public alt.knowledge.loop API exposes only
-- surface_planner_version.

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
  surface_score_inputs
FROM knowledge_loop_entries;

COMMENT ON VIEW knowledge_loop_entries_public IS
  'Read-handler-facing view that intentionally excludes projected_at. All Get/Stream handlers MUST read through this view; direct SELECT on knowledge_loop_entries is projector-only.';
