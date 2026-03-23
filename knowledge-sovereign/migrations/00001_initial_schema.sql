-- Knowledge Sovereign: initial schema
-- Consolidated from alt-db migrations 20260318..20260323.
-- All Sovereign-owned tables in a single file.

--------------------------------------------------------------------------------
-- 1. knowledge_events  (append-only event store, CQRS write side)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_events (
  event_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_seq       BIGSERIAL UNIQUE NOT NULL,
  occurred_at     TIMESTAMPTZ NOT NULL,
  tenant_id       UUID NOT NULL,
  user_id         UUID,
  actor_type      TEXT NOT NULL,
  actor_id        TEXT,
  event_type      TEXT NOT NULL,
  aggregate_type  TEXT NOT NULL,
  aggregate_id    TEXT NOT NULL,
  correlation_id  UUID,
  causation_id    UUID,
  dedupe_key      TEXT NOT NULL UNIQUE,
  payload         JSONB NOT NULL
);

CREATE INDEX idx_knowledge_events_aggregate
  ON knowledge_events (aggregate_type, aggregate_id, event_seq);
CREATE INDEX idx_knowledge_events_occurred
  ON knowledge_events (occurred_at DESC);
CREATE INDEX idx_knowledge_events_type_seq
  ON knowledge_events (event_type, event_seq);
CREATE INDEX idx_ke_seq_type
  ON knowledge_events (event_seq, event_type);

COMMENT ON TABLE knowledge_events IS 'Knowledge Home append-only event log (CQRS write side)';

--------------------------------------------------------------------------------
-- 2. knowledge_user_events  (user interaction log)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_user_events (
  user_event_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  occurred_at    TIMESTAMPTZ NOT NULL,
  user_id        UUID NOT NULL,
  tenant_id      UUID NOT NULL,
  event_type     TEXT NOT NULL,
  item_key       TEXT NOT NULL,
  payload        JSONB NOT NULL DEFAULT '{}',
  dedupe_key     TEXT
);

CREATE INDEX idx_knowledge_user_events_user_time
  ON knowledge_user_events (user_id, occurred_at DESC);
CREATE UNIQUE INDEX idx_knowledge_user_events_dedupe
  ON knowledge_user_events (dedupe_key) WHERE dedupe_key IS NOT NULL;
CREATE INDEX idx_kue_user_occurred
  ON knowledge_user_events (user_id, occurred_at DESC);

COMMENT ON TABLE knowledge_user_events IS 'User interaction log for Knowledge Home';

--------------------------------------------------------------------------------
-- 3. knowledge_home_items  (CQRS read-model projection, rebuildable)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_home_items (
  user_id            UUID NOT NULL,
  tenant_id          UUID NOT NULL,
  item_key           TEXT NOT NULL,
  item_type          TEXT NOT NULL,
  primary_ref_id     UUID,
  title              TEXT NOT NULL,
  summary_excerpt    TEXT,
  tags_json          JSONB NOT NULL DEFAULT '[]',
  why_json           JSONB NOT NULL DEFAULT '[]',
  score              NUMERIC NOT NULL DEFAULT 0,
  freshness_at       TIMESTAMPTZ,
  published_at       TIMESTAMPTZ,
  last_interacted_at TIMESTAMPTZ,
  generated_at       TIMESTAMPTZ NOT NULL,
  updated_at         TIMESTAMPTZ NOT NULL,
  projection_version INTEGER NOT NULL DEFAULT 1,
  supersede_state    TEXT,
  superseded_at      TIMESTAMPTZ,
  previous_ref_json  JSONB,
  summary_state      TEXT NOT NULL DEFAULT 'missing',
  dismissed_at       TIMESTAMPTZ,
  PRIMARY KEY (user_id, item_key, projection_version)
);

CREATE INDEX idx_kh_items_user_score
  ON knowledge_home_items (user_id, score DESC, published_at DESC);
CREATE INDEX idx_kh_items_version
  ON knowledge_home_items (projection_version);
CREATE INDEX idx_kh_items_supersede
  ON knowledge_home_items (user_id, supersede_state) WHERE supersede_state IS NOT NULL;
CREATE INDEX idx_khi_user_version_score
  ON knowledge_home_items (user_id, projection_version, score DESC, published_at DESC);
CREATE INDEX idx_kh_items_user_version_visible_score
  ON knowledge_home_items (user_id, projection_version, score DESC, published_at DESC)
  WHERE dismissed_at IS NULL;

COMMENT ON TABLE knowledge_home_items IS 'Knowledge Home read model (CQRS projection, rebuildable)';

--------------------------------------------------------------------------------
-- 4. today_digest_view  (daily digest projection for TodayBar)
--------------------------------------------------------------------------------
CREATE TABLE today_digest_view (
  user_id               UUID NOT NULL,
  digest_date           DATE NOT NULL,
  new_articles          INTEGER NOT NULL DEFAULT 0,
  summarized_articles   INTEGER NOT NULL DEFAULT 0,
  unsummarized_articles INTEGER NOT NULL DEFAULT 0,
  top_tags_json         JSONB NOT NULL DEFAULT '[]',
  pulse_refs_json       JSONB NOT NULL DEFAULT '[]',
  updated_at            TIMESTAMPTZ NOT NULL,
  projection_version    INTEGER NOT NULL DEFAULT 1,
  weekly_recap_available  BOOLEAN NOT NULL DEFAULT false,
  evening_pulse_available BOOLEAN NOT NULL DEFAULT false,
  PRIMARY KEY (user_id, digest_date)
);

COMMENT ON TABLE today_digest_view IS 'Daily digest projection for TodayBar';
COMMENT ON COLUMN today_digest_view.weekly_recap_available IS 'Backend-authoritative flag for weekly recap CTA';
COMMENT ON COLUMN today_digest_view.evening_pulse_available IS 'Backend-authoritative flag for evening pulse CTA';

--------------------------------------------------------------------------------
-- 5. recall_candidate_view  (recall candidates projection)
--------------------------------------------------------------------------------
CREATE TABLE recall_candidate_view (
  user_id           UUID NOT NULL,
  item_key          TEXT NOT NULL,
  recall_score      NUMERIC NOT NULL DEFAULT 0,
  reason_json       JSONB NOT NULL DEFAULT '[]',
  next_suggest_at   TIMESTAMPTZ,
  updated_at        TIMESTAMPTZ NOT NULL,
  projection_version INTEGER NOT NULL DEFAULT 1,
  first_eligible_at TIMESTAMPTZ,
  snoozed_until     TIMESTAMPTZ,
  PRIMARY KEY (user_id, item_key)
);

CREATE INDEX idx_recall_candidates_suggest
  ON recall_candidate_view (user_id, next_suggest_at ASC)
  WHERE next_suggest_at IS NOT NULL AND snoozed_until IS NULL;
CREATE INDEX idx_rcv_user_version_suggest
  ON recall_candidate_view (user_id, projection_version, next_suggest_at ASC)
  WHERE next_suggest_at IS NOT NULL AND snoozed_until IS NULL;

COMMENT ON TABLE recall_candidate_view IS 'Recall candidates projection (Phase 1: skeleton only)';

--------------------------------------------------------------------------------
-- 6. recall_signals  (user interaction signals for recall scoring)
--------------------------------------------------------------------------------
CREATE TABLE recall_signals (
  signal_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL,
  item_key        TEXT NOT NULL,
  signal_type     TEXT NOT NULL,
  signal_strength NUMERIC NOT NULL DEFAULT 1.0,
  occurred_at     TIMESTAMPTZ NOT NULL,
  payload         JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_recall_signals_user_time ON recall_signals (user_id, occurred_at DESC);
CREATE INDEX idx_recall_signals_item ON recall_signals (user_id, item_key, signal_type);

COMMENT ON TABLE recall_signals IS 'Tracks user interaction signals for recall scoring (Phase 4)';

--------------------------------------------------------------------------------
-- 7. knowledge_projection_checkpoints  (projector incremental cursors)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_projection_checkpoints (
  projector_name TEXT PRIMARY KEY,
  last_event_seq BIGINT NOT NULL DEFAULT 0,
  updated_at     TIMESTAMPTZ NOT NULL
);

COMMENT ON TABLE knowledge_projection_checkpoints IS 'Projector checkpoint for incremental event processing';

--------------------------------------------------------------------------------
-- 8. knowledge_projection_versions  (projection version registry)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_projection_versions (
  version       INTEGER PRIMARY KEY,
  description   TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'pending',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  activated_at  TIMESTAMPTZ
);

INSERT INTO knowledge_projection_versions (version, description, status, activated_at)
VALUES (1, 'Initial Knowledge Home projection', 'active', now());

--------------------------------------------------------------------------------
-- 9. knowledge_backfill_jobs  (backfill job tracking)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_backfill_jobs (
  job_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  status              TEXT NOT NULL DEFAULT 'pending',
  projection_version  INTEGER NOT NULL,
  cursor_user_id      UUID,
  cursor_date         DATE,
  cursor_article_id   UUID,
  total_events        INTEGER NOT NULL DEFAULT 0,
  processed_events    INTEGER NOT NULL DEFAULT 0,
  error_message       TEXT,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at          TIMESTAMPTZ,
  completed_at        TIMESTAMPTZ,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_backfill_jobs_status ON knowledge_backfill_jobs (status);

--------------------------------------------------------------------------------
-- 10. knowledge_reproject_runs  (re-projection run tracking)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_reproject_runs (
  reproject_run_id    UUID PRIMARY KEY,
  projection_name     TEXT NOT NULL,
  from_version        TEXT NOT NULL,
  to_version          TEXT NOT NULL,
  initiated_by        UUID,
  mode                TEXT NOT NULL, -- full / time_range / user_subset / dry_run
  status              TEXT NOT NULL, -- pending / running / validating / swappable / swapped / failed / cancelled
  range_start         TIMESTAMPTZ,
  range_end           TIMESTAMPTZ,
  checkpoint_payload  JSONB NOT NULL DEFAULT '{}',
  stats_json          JSONB NOT NULL DEFAULT '{}',
  diff_summary_json   JSONB NOT NULL DEFAULT '{}',
  created_at          TIMESTAMPTZ NOT NULL,
  started_at          TIMESTAMPTZ,
  finished_at         TIMESTAMPTZ
);

CREATE INDEX idx_reproject_runs_status ON knowledge_reproject_runs (status, created_at DESC);

--------------------------------------------------------------------------------
-- 11. knowledge_projection_audits  (projection audit results)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_projection_audits (
  audit_id            UUID PRIMARY KEY,
  projection_name     TEXT NOT NULL,
  projection_version  TEXT NOT NULL,
  checked_at          TIMESTAMPTZ NOT NULL,
  sample_size         INTEGER NOT NULL,
  mismatch_count      INTEGER NOT NULL,
  details_json        JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_projection_audits_name ON knowledge_projection_audits (projection_name, checked_at DESC);

--------------------------------------------------------------------------------
-- 12. knowledge_lenses  (saved viewpoints + versioned filter configs)
--------------------------------------------------------------------------------
CREATE TABLE knowledge_lenses (
  lens_id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL,
  tenant_id   UUID NOT NULL,
  name        TEXT NOT NULL,
  description TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at TIMESTAMPTZ
);

CREATE INDEX idx_knowledge_lenses_user ON knowledge_lenses (user_id) WHERE archived_at IS NULL;

CREATE TABLE knowledge_lens_versions (
  lens_version_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  lens_id          UUID NOT NULL REFERENCES knowledge_lenses(lens_id),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  query_text       TEXT,
  tag_ids_json     JSONB NOT NULL DEFAULT '[]',
  time_window_json JSONB,
  include_recap    BOOLEAN NOT NULL DEFAULT true,
  include_pulse    BOOLEAN NOT NULL DEFAULT true,
  sort_mode        TEXT NOT NULL DEFAULT 'relevance',
  superseded_by    UUID REFERENCES knowledge_lens_versions(lens_version_id),
  source_ids_json  JSONB NOT NULL DEFAULT '[]'
);

CREATE TABLE knowledge_current_lens (
  user_id         UUID PRIMARY KEY,
  lens_id         UUID NOT NULL REFERENCES knowledge_lenses(lens_id),
  lens_version_id UUID NOT NULL REFERENCES knowledge_lens_versions(lens_version_id),
  selected_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
