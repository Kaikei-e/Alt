-- Knowledge Loop read model (CQRS projection, rebuildable from knowledge_events).
-- Append-first / reproject-safe / versioned-artifact / disposable-projection invariants hold.
-- updated_at is intentionally absent; use projection_seq_hiwater / projection_revision /
-- freshness_at / source_observed_at / artifact_version_ref instead.
-- projected_at is wall-clock for operational debug only; it MUST NOT be exposed via
-- API / proto / public view / metrics / production logs.

-- ============================================================================
-- enums
-- ============================================================================

CREATE TYPE loop_stage AS ENUM (
  'observe',
  'orient',
  'decide',
  'act'
);

CREATE TYPE surface_bucket AS ENUM (
  'now',
  'continue',
  'changed',
  'review'
);

CREATE TYPE dismiss_state AS ENUM (
  'active',
  'deferred',
  'dismissed',
  'completed'
);

CREATE TYPE why_kind AS ENUM (
  'source_why',
  'pattern_why',
  'recall_why',
  'change_why'
);

CREATE TYPE loop_priority AS ENUM (
  'critical',
  'continuing',
  'confirm',
  'reference'
);

CREATE TYPE loop_service_quality AS ENUM (
  'full',
  'degraded',
  'fallback'
);

-- ============================================================================
-- knowledge_loop_entries
-- ============================================================================

CREATE TABLE knowledge_loop_entries (
  user_id                      UUID            NOT NULL,
  tenant_id                    UUID            NOT NULL,
  lens_mode_id                 TEXT            NOT NULL,
  entry_key                    TEXT            NOT NULL,
  source_item_key              TEXT            NOT NULL,

  proposed_stage               loop_stage      NOT NULL,
  surface_bucket               surface_bucket  NOT NULL,

  projection_revision          BIGINT          NOT NULL DEFAULT 1,
  projection_seq_hiwater       BIGINT          NOT NULL,
  source_event_seq             BIGINT          NOT NULL,

  freshness_at                 TIMESTAMPTZ     NOT NULL,
  source_observed_at           TIMESTAMPTZ,
  projected_at                 TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

  artifact_summary_version_id  TEXT,
  artifact_tag_set_version_id  TEXT,
  artifact_lens_version_id     TEXT,
  CONSTRAINT kle_artifact_version_ref_not_all_null CHECK (
    artifact_summary_version_id IS NOT NULL OR
    artifact_tag_set_version_id IS NOT NULL OR
    artifact_lens_version_id    IS NOT NULL
  ),

  why_kind                     why_kind        NOT NULL,
  why_text                     TEXT            NOT NULL
                              CHECK (length(why_text) BETWEEN 1 AND 512),
  why_confidence               REAL
                              CHECK (why_confidence IS NULL OR (why_confidence >= 0.0 AND why_confidence <= 1.0)),
  why_evidence_ref_ids         TEXT[]          NOT NULL DEFAULT ARRAY[]::TEXT[],
  CONSTRAINT kle_why_evidence_ref_ids_bounded CHECK (
    array_length(why_evidence_ref_ids, 1) IS NULL
    OR array_length(why_evidence_ref_ids, 1) <= 8
  ),
  why_evidence_refs            JSONB           NOT NULL DEFAULT '[]'::jsonb,
  CONSTRAINT kle_why_evidence_refs_bounded CHECK (
    jsonb_typeof(why_evidence_refs) = 'array'
    AND jsonb_array_length(why_evidence_refs) <= 8
  ),

  change_summary               JSONB,
  continue_context             JSONB,
  decision_options             JSONB,
  act_targets                  JSONB,

  superseded_by_entry_key      TEXT,
  dismiss_state                dismiss_state   NOT NULL DEFAULT 'active',

  render_depth_hint            SMALLINT        NOT NULL DEFAULT 1
                              CHECK (render_depth_hint BETWEEN 1 AND 4),

  loop_priority                loop_priority   NOT NULL,

  CONSTRAINT kle_entry_key_format        CHECK (entry_key        ~ '^[A-Za-z0-9_:-]{1,128}$'),
  CONSTRAINT kle_source_item_key_format  CHECK (source_item_key  ~ '^[A-Za-z0-9_:-]{1,128}$'),
  CONSTRAINT kle_lens_mode_id_format     CHECK (lens_mode_id     ~ '^[A-Za-z0-9_:-]{1,128}$'),

  PRIMARY KEY (user_id, lens_mode_id, entry_key)
);

CREATE INDEX idx_kle_surface
  ON knowledge_loop_entries (user_id, lens_mode_id, surface_bucket, projection_seq_hiwater DESC);

CREATE INDEX idx_kle_stage
  ON knowledge_loop_entries (user_id, lens_mode_id, proposed_stage, projection_seq_hiwater DESC);

CREATE INDEX idx_kle_why_kind
  ON knowledge_loop_entries (user_id, lens_mode_id, why_kind, projection_seq_hiwater DESC);

CREATE INDEX idx_kle_active_freshness
  ON knowledge_loop_entries (user_id, lens_mode_id, freshness_at DESC)
  WHERE dismiss_state = 'active';

CREATE INDEX idx_kle_superseded
  ON knowledge_loop_entries (user_id, lens_mode_id, superseded_by_entry_key)
  WHERE superseded_by_entry_key IS NOT NULL;

COMMENT ON TABLE knowledge_loop_entries IS
  'Knowledge Loop entry projection. Reprojectable from knowledge_events. No updated_at by design.';
COMMENT ON COLUMN knowledge_loop_entries.projected_at IS
  'Internal debugging only. MUST NOT be exposed via API, proto, public view, metrics, or production logs.';
COMMENT ON COLUMN knowledge_loop_entries.freshness_at IS
  'MAX(occurred_at) across reflected events. Business-fact time, never wall-clock.';

-- ============================================================================
-- knowledge_loop_entries_public — read-handler-facing view (excludes projected_at)
-- ============================================================================

CREATE VIEW knowledge_loop_entries_public AS
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
  loop_priority
FROM knowledge_loop_entries;

COMMENT ON VIEW knowledge_loop_entries_public IS
  'Read-handler-facing view that intentionally excludes projected_at. All Get/Stream handlers MUST read through this view; direct SELECT on knowledge_loop_entries is projector-only.';

-- ============================================================================
-- knowledge_loop_session_state
-- ============================================================================

CREATE TABLE knowledge_loop_session_state (
  user_id                       UUID          NOT NULL,
  tenant_id                     UUID          NOT NULL,
  lens_mode_id                  TEXT          NOT NULL,

  current_stage                 loop_stage    NOT NULL,
  current_stage_entered_at      TIMESTAMPTZ   NOT NULL,

  focused_entry_key             TEXT,
  foreground_entry_key          TEXT,

  last_observed_entry_key       TEXT,
  last_oriented_entry_key       TEXT,
  last_decided_entry_key        TEXT,
  last_acted_entry_key          TEXT,
  last_returned_entry_key       TEXT,
  last_deferred_entry_key       TEXT,

  projection_revision           BIGINT        NOT NULL DEFAULT 1,
  projection_seq_hiwater        BIGINT        NOT NULL,
  projected_at                  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),

  CONSTRAINT klss_lens_mode_id_format CHECK (lens_mode_id ~ '^[A-Za-z0-9_:-]{1,128}$'),

  PRIMARY KEY (user_id, lens_mode_id)
);

COMMENT ON TABLE knowledge_loop_session_state IS
  'Per-user-per-lens Loop session state. current_stage_entered_at MUST come from the triggering event occurred_at, never wall-clock.';
COMMENT ON COLUMN knowledge_loop_session_state.focused_entry_key IS
  'Accessibility focus: keyboard or screen-reader focus. May differ from foreground_entry_key.';
COMMENT ON COLUMN knowledge_loop_session_state.foreground_entry_key IS
  'Foreground primary entry. Subject to optimistic lock via projection_revision.';
COMMENT ON COLUMN knowledge_loop_session_state.projected_at IS
  'Internal debugging only. MUST NOT be exposed.';

-- ============================================================================
-- knowledge_loop_surfaces
-- ============================================================================

CREATE TABLE knowledge_loop_surfaces (
  user_id                      UUID                 NOT NULL,
  tenant_id                    UUID                 NOT NULL,
  lens_mode_id                 TEXT                 NOT NULL,
  surface_bucket               surface_bucket       NOT NULL,

  primary_entry_key            TEXT,
  secondary_entry_keys         TEXT[]               NOT NULL DEFAULT ARRAY[]::TEXT[],
  CONSTRAINT kls_secondary_entry_keys_limit CHECK (
    array_length(secondary_entry_keys, 1) IS NULL OR array_length(secondary_entry_keys, 1) <= 2
  ),

  projection_revision          BIGINT               NOT NULL DEFAULT 1,
  projection_seq_hiwater       BIGINT               NOT NULL,
  freshness_at                 TIMESTAMPTZ          NOT NULL,
  projected_at                 TIMESTAMPTZ          NOT NULL DEFAULT NOW(),

  service_quality              loop_service_quality NOT NULL,
  loop_health                  JSONB                NOT NULL DEFAULT '{}'::jsonb,

  CONSTRAINT kls_lens_mode_id_format CHECK (lens_mode_id ~ '^[A-Za-z0-9_:-]{1,128}$'),

  PRIMARY KEY (user_id, lens_mode_id, surface_bucket)
);

COMMENT ON TABLE knowledge_loop_surfaces IS
  'Per-surface bucket summary for the Loop UI (foreground/continue/changed/review).';

-- ============================================================================
-- knowledge_loop_transition_dedupes — ingest-side idempotency barrier (NOT a projection)
-- ============================================================================

CREATE TABLE knowledge_loop_transition_dedupes (
  user_id                      UUID          NOT NULL,
  client_transition_id         TEXT          NOT NULL,
  canonical_entry_key          TEXT,
  response_payload             JSONB,
  created_at                   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, client_transition_id)
);

CREATE INDEX idx_klt_dedupe_created_at
  ON knowledge_loop_transition_dedupes (created_at);

COMMENT ON TABLE knowledge_loop_transition_dedupes IS
  'Ingest-side idempotency barrier keyed by (user_id, client_transition_id). NOT a projection — full reproject leaves this table untouched. TTL 48h managed by worker.';

-- ============================================================================
-- dangling_supersede_refs — projection integrity check
-- ============================================================================

CREATE VIEW dangling_supersede_refs AS
SELECT
  e.user_id,
  e.lens_mode_id,
  e.entry_key,
  e.superseded_by_entry_key AS missing_target,
  e.projected_at
FROM knowledge_loop_entries e
LEFT JOIN knowledge_loop_entries t
  ON  e.user_id                 = t.user_id
  AND e.lens_mode_id            = t.lens_mode_id
  AND e.superseded_by_entry_key = t.entry_key
WHERE e.superseded_by_entry_key IS NOT NULL
  AND t.entry_key IS NULL;

COMMENT ON VIEW dangling_supersede_refs IS
  'Projection integrity check for Knowledge Loop supersede chain. Non-zero rows indicate dangling references; investigate via runbook knowledge-loop-reproject.md.';
