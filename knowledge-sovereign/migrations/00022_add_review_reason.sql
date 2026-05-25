-- Epistemic-change review driver (ADR-000907).
--
-- review_reason is a projector-owned column populated by decideReviewReason
-- when an entry lands in Review. NULL is invalid — every row gets at least
-- 'none' so the read path can rely on a non-null value. The CHECK locks the
-- value set so an accidental projector emit cannot widen the vocabulary
-- silently.
--
-- Reproject-safety: decideReviewReason is a pure function of
-- SurfaceScoreInputs. Replay produces the same reason for the same event log.
--
-- Merge-safety: UPSERT path overwrites the column from EXCLUDED, not COALESCE,
-- because the projector is the only writer.

ALTER TABLE knowledge_loop_entries
  ADD COLUMN review_reason TEXT NOT NULL DEFAULT 'none';

ALTER TABLE knowledge_loop_entries
  ADD CONSTRAINT kle_review_reason_values CHECK (
    review_reason IN ('staleness', 'contradiction', 'version_drift', 'unfinished_thread', 'none')
  );

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
  review_reason
FROM knowledge_loop_entries;

COMMENT ON COLUMN knowledge_loop_entries.review_reason IS
  'ADR-000907 epistemic-change review driver. Pure function of SurfaceScoreInputs; NULL is invalid — non-Review entries store ''none''.';
