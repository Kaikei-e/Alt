-- Macro (day-to-week) projection for Knowledge Loop session state.
--
-- ADR-000909 §Δ2 supplement (2026-05-24): persists the multi-scale loop
-- overview surfaced by MacroByline on /loop. The projector recomputes this
-- row from the event log whenever an Acted / Reviewed / ActOutcome event
-- lands; the read path joins the row into the GetKnowledgeLoop response so
-- Lens switches show real counts instead of always-zero placeholders.
--
-- Reproject-safety:
--   - Every count derives from knowledge_events payload — no wall-clock,
--     no latest-state queries (see macro_state_builder.go).
--   - window_end_at echoes the triggering event's occurred_at; the 7-day
--     window left edge is implicit (window_end_at - 7d).
--   - seq_hiwater is the merge-safe upsert guard: replays whose
--     EXCLUDED.seq_hiwater <= projection.seq_hiwater are no-ops.
--   - lens_weights_version pins the cohort that produced cognitive_load_hint.
--     A bump on the application side triggers a full reproject via the
--     knowledge-loop-reproject runbook.
--
-- The table is a disposable projection — it can be truncated and rebuilt
-- from the event log at any time.

CREATE TYPE knowledge_loop_cognitive_load_hint AS ENUM (
  'unspecified',
  'light',
  'medium',
  'heavy'
);

CREATE TABLE knowledge_loop_macro_state (
  user_id                   UUID                              NOT NULL,
  tenant_id                 UUID                              NOT NULL,
  lens_mode_id              TEXT                              NOT NULL,

  active_continue_threads   INTEGER                           NOT NULL DEFAULT 0,
  pending_review_count      INTEGER                           NOT NULL DEFAULT 0,
  recent_internalized_count INTEGER                           NOT NULL DEFAULT 0,
  cognitive_load_hint       knowledge_loop_cognitive_load_hint NOT NULL DEFAULT 'unspecified',

  window_start_at           TIMESTAMPTZ                       NOT NULL,
  window_end_at             TIMESTAMPTZ                       NOT NULL,
  seq_hiwater               BIGINT                            NOT NULL,
  lens_weights_version      INTEGER                           NOT NULL,

  projected_at              TIMESTAMPTZ                       NOT NULL DEFAULT NOW(),

  CONSTRAINT klms_lens_mode_id_format
    CHECK (lens_mode_id ~ '^[A-Za-z0-9_:-]{1,128}$'),
  CONSTRAINT klms_counts_nonneg
    CHECK (active_continue_threads >= 0
       AND pending_review_count >= 0
       AND recent_internalized_count >= 0),
  CONSTRAINT klms_window_ordered
    CHECK (window_start_at < window_end_at),

  PRIMARY KEY (user_id, tenant_id, lens_mode_id)
);

CREATE INDEX idx_klms_user_lens_seq
  ON knowledge_loop_macro_state
  (user_id, tenant_id, lens_mode_id, seq_hiwater DESC);

COMMENT ON TABLE knowledge_loop_macro_state IS
  'ADR-000909 §Δ2 — disposable projection of the user macro (7d) cognitive footprint. Reproject-safe: every count derives from event payload only.';
COMMENT ON COLUMN knowledge_loop_macro_state.window_end_at IS
  'Echoes the triggering event occurred_at. Never wall-clock.';
COMMENT ON COLUMN knowledge_loop_macro_state.seq_hiwater IS
  'Merge-safe upsert guard. Replays whose EXCLUDED value is not greater are no-ops.';
COMMENT ON COLUMN knowledge_loop_macro_state.lens_weights_version IS
  'Pins the lens weights cohort used to compute cognitive_load_hint. Bumping triggers full reproject.';
COMMENT ON COLUMN knowledge_loop_macro_state.projected_at IS
  'Internal debugging only. MUST NOT be exposed in API responses.';

ALTER TABLE knowledge_loop_macro_state ENABLE ROW LEVEL SECURITY;

CREATE POLICY knowledge_loop_macro_state_user_isolation
  ON knowledge_loop_macro_state
  USING (user_id::text = current_setting('alt.user_id', true));
