-- ADR-000939: co-projected evidence accumulator for the Knowledge Loop.
--
-- Replaces the EventLogSurfaceScoreResolver's per-entry 7-day window re-scan
-- (which truncated at LIMIT 256 and silently lost all evidence at production
-- log density) with an append-time accumulator the projector updates O(1) per
-- event. SurfaceScoreInputs are then a pure derivation over this table instead
-- of a pull-time scan.
--
-- Co-projection contract (ADR-000939 §2): this table is written ONLY by the
-- knowledge-loop-projector, in the same ordered pass as knowledge_loop_entries,
-- under the same `knowledge-loop-projector` checkpoint. The projector derives
-- an entry from the accumulator state of the event-log prefix (seq < N) and
-- then applies event N's fact, so an entry's relations stay a deterministic
-- pure function of the prefix and reproject reproduces them bit-for-bit.
--
-- Disposable: a full reproject TRUNCATEs this table together with
-- knowledge_loop_entries and rebuilds it from the event log. No updated_at, no
-- wall-clock column — every timestamp is event-time (occurred_at). Windowed
-- counts are derived from the raw `facts` (each carries occurred_at + seq) at
-- derivation time; no precomputed decay is stored (canonical contract §6.5).

CREATE TABLE knowledge_loop_evidence (
  user_id          UUID        NOT NULL,
  tenant_id        UUID        NOT NULL,
  -- scope_kind/scope_ref name what the evidence is about:
  --   entry      → a knowledge_loop entry_key (per-entry interaction signals)
  --   article    → an article_id (version drift / supersede / url / current tags)
  --   tag         → a tag name the user is active on
  --   topic_term → a recap topic top_term the user's recaps surfaced
  scope_kind       TEXT        NOT NULL CHECK (scope_kind IN ('entry', 'article', 'tag', 'topic_term')),
  scope_ref        TEXT        NOT NULL CHECK (length(scope_ref) BETWEEN 1 AND 256),
  signal_kind      TEXT        NOT NULL CHECK (signal_kind IN (
                     'summary_version', 'summary_supersede', 'open_interaction',
                     'continue_act', 'compare_act', 'act_outcome', 'augur_link',
                     'tag_activity', 'topic_snapshot', 'tag_set_current',
                     'url_pin', 'article_pin')),
  -- Bounded ring of raw event-time facts (newest 32 by event_seq). Each element
  -- is {"occurred_at": <rfc3339>, "event_seq": <int>, "v": <opaque string>}.
  -- Windowed counts (7d, event-time bound) are derived from this at derivation
  -- time; the ring saturates magnitude at 32, which is well above any relation
  -- magnitude the Orient surface renders. NOT used by pin-style signals
  -- (url_pin / article_pin / tag_set_current) which carry their latest value in
  -- pinned_text / pinned_payload instead.
  facts            JSONB       NOT NULL DEFAULT '[]'::jsonb
                   CHECK (jsonb_typeof(facts) = 'array' AND jsonb_array_length(facts) <= 32),
  -- Lifetime count for audit/observability only. Never used for windowed
  -- derivation (that would be a precomputed aggregate over an unbounded window).
  facts_total      BIGINT      NOT NULL DEFAULT 0,
  -- Latest pinned scalar (url_pin → http(s) url, article_pin → article_id).
  pinned_text      TEXT,
  -- Latest pinned structured value (tag_set_current → {"tags": [...]}).
  pinned_payload   JSONB,
  -- MAX(occurred_at) across applied facts. Event-time only; never wall-clock.
  last_occurred_at TIMESTAMPTZ NOT NULL,
  -- Merge-safety guard: an UPSERT only advances state when it carries a higher
  -- event_seq, so replaying the same event (or an out-of-order delivery) is a
  -- no-op. This is what makes the accumulator reproject-deterministic.
  last_event_seq   BIGINT      NOT NULL,
  PRIMARY KEY (user_id, scope_kind, scope_ref, signal_kind)
);

COMMENT ON TABLE knowledge_loop_evidence IS
  'ADR-000939 co-projected evidence accumulator. Disposable; TRUNCATEd on every reproject together with knowledge_loop_entries. Single writer: knowledge-loop-projector. No updated_at / no wall-clock by design — raw event-time facts only, windows derived at read time.';

-- The PK btree (user_id, scope_kind, scope_ref, signal_kind) also serves the
-- prefix lookups the projector's Derive does (user_id + scope_kind + scope_ref,
-- including scope_ref = ANY(...) for the article's tag/topic_term fan-in), so no
-- additional index is needed.

-- F-001 defense-in-depth, mirroring knowledge_loop_entries (migration 00014).
-- The projector writes/reads as the table owner, which bypasses RLS (the policy
-- is ENABLE, not FORCE), and Derive binds user_id physically in every query;
-- the policy protects any future non-owner read role.
ALTER TABLE knowledge_loop_evidence ENABLE ROW LEVEL SECURITY;

CREATE POLICY knowledge_loop_evidence_user_isolation
  ON knowledge_loop_evidence
  USING (user_id::text = current_setting('alt.user_id', true));

COMMENT ON POLICY knowledge_loop_evidence_user_isolation ON knowledge_loop_evidence IS
  'F-001 defense-in-depth. Each non-owner session must SET LOCAL alt.user_id = $user_id before reading.';
