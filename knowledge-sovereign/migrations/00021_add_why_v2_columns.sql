-- WhyPayload v2 producer fields (ADR-000908 §Δ4).
--
-- Three additive columns extend knowledge_loop_entries so the projector can
-- persist counter_evidence_refs, confidence_ladder, and what_would_change_my_mind
-- alongside the existing why_* columns. Existing rows pick up the defaults
-- ('[]' for refs, NULL for ladder/WWCM) and converge to populated values on the
-- next WhyMappingVersion=11 reproject.
--
-- Reproject-safety: every value is a pure function of the WhyKind (see
-- why_v2_helpers.go), so the same event log yields the same projection rows.
--
-- Merge-safety: the UPSERT path overwrites these columns from EXCLUDED, never
-- COALESCE — they are projector-owned and reproject must be able to replace
-- them.
--
-- Disposable projections: knowledge_loop_entries is rebuildable from the
-- event log; this migration is additive only and never drops data.

ALTER TABLE knowledge_loop_entries
  ADD COLUMN why_counter_evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN why_confidence_ladder TEXT NULL,
  ADD COLUMN why_what_would_change_my_mind TEXT NULL;

-- Caps and value-set guards mirror canonical contract §11:
--   - counter_evidence_refs length <= 4
--   - confidence_ladder ∈ {speculation, pattern, evidence, verified}
--   - what_would_change_my_mind length 1..256 when present

ALTER TABLE knowledge_loop_entries
  ADD CONSTRAINT kle_why_counter_evidence_refs_cap
    CHECK (jsonb_array_length(why_counter_evidence_refs) <= 4);

ALTER TABLE knowledge_loop_entries
  ADD CONSTRAINT kle_why_confidence_ladder_values
    CHECK (
      why_confidence_ladder IS NULL OR
      why_confidence_ladder IN ('speculation', 'pattern', 'evidence', 'verified')
    );

ALTER TABLE knowledge_loop_entries
  ADD CONSTRAINT kle_why_what_would_change_length
    CHECK (
      why_what_would_change_my_mind IS NULL OR
      (length(why_what_would_change_my_mind) BETWEEN 1 AND 256)
    );

-- CREATE OR REPLACE VIEW only permits appending columns at the tail. The
-- existing column list (locked since migration 00018) ends with
-- visibility_state, completion_state; the v2 columns are appended after
-- those. Read clients select by name, so order does not change parsing.

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
  why_what_would_change_my_mind
FROM knowledge_loop_entries;

COMMENT ON COLUMN knowledge_loop_entries.why_counter_evidence_refs IS
  'WhyPayload v2 counter-evidence refs (length <= 4). Reproject-safe: derived from event payload only.';
COMMENT ON COLUMN knowledge_loop_entries.why_confidence_ladder IS
  'WhyPayload v2 confidence ladder tier (speculation/pattern/evidence/verified). NULL until projector v11 populates.';
COMMENT ON COLUMN knowledge_loop_entries.why_what_would_change_my_mind IS
  'WhyPayload v2 falsifier sentence (1..256 chars). NULL when WhyKind is unspecified or carries no claim.';
