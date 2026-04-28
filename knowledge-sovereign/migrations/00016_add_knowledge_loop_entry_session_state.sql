-- Per-entry user progress for Knowledge Loop.
--
-- proposed_stage on knowledge_loop_entries is the projector recommendation.
-- This table records where a given user currently is with a given entry so
-- replay and UI refresh do not mutate proposed_stage as session progress.

CREATE TABLE knowledge_loop_entry_session_state (
  user_id                       UUID          NOT NULL,
  tenant_id                     UUID          NOT NULL,
  lens_mode_id                  TEXT          NOT NULL,
  entry_key                     TEXT          NOT NULL,

  current_stage                 loop_stage    NOT NULL,
  current_stage_entered_at      TIMESTAMPTZ   NOT NULL,

  projection_revision           BIGINT        NOT NULL DEFAULT 1,
  projection_seq_hiwater        BIGINT        NOT NULL,
  projected_at                  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),

  CONSTRAINT kless_entry_key_format    CHECK (entry_key    ~ '^[A-Za-z0-9_:-]{1,128}$'),
  CONSTRAINT kless_lens_mode_id_format CHECK (lens_mode_id ~ '^[A-Za-z0-9_:-]{1,128}$'),

  PRIMARY KEY (user_id, tenant_id, lens_mode_id, entry_key)
);

CREATE INDEX idx_kless_user_lens_seq
  ON knowledge_loop_entry_session_state (user_id, tenant_id, lens_mode_id, projection_seq_hiwater DESC);

COMMENT ON TABLE knowledge_loop_entry_session_state IS
  'Per-user-per-entry Loop progress. current_stage_entered_at MUST come from the triggering event occurred_at, never wall-clock.';
COMMENT ON COLUMN knowledge_loop_entry_session_state.projected_at IS
  'Internal debugging only. MUST NOT be exposed.';

ALTER TABLE knowledge_loop_entry_session_state ENABLE ROW LEVEL SECURITY;

CREATE POLICY knowledge_loop_entry_session_state_user_isolation
  ON knowledge_loop_entry_session_state
  USING (user_id::text = current_setting('alt.user_id', true));
