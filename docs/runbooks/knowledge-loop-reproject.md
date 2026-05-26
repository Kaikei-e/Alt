---
title: Knowledge Loop Reproject Runbook
date: 2026-04-23
status: accepted
tags:
  - runbook
  - knowledge-loop
  - projection
  - reproject
aliases:
  - knowledge-loop-reproject
---

# Knowledge Loop Reproject Runbook

ADR: [[000831]]  
Canonical contract: [[knowledge-loop-canonical-contract]]

Full-reproject procedure for the Knowledge Loop read model. Use this when:

- the `WhyMappingVersion` constant in `alt-backend/app/usecase/knowledge_loop_usecase/validator.go` bumps
- the `SurfacePlannerVersion` enum value used by the projector is bumped (canonical contract §6.5)
- the projector emit rules change in `job/knowledge_loop_projector.go`
- the `dangling_supersede_refs` view is non-empty
- a migration modifies the projection schema in a non-idempotent way

## Pre-flight

1. Confirm the write side is healthy.
   ```sql
   SELECT COUNT(*) FROM knowledge_events;
   SELECT MAX(event_seq) FROM knowledge_events;
   ```
2. Confirm the checkpoint row exists.
   ```sql
   SELECT projector_name, last_event_seq, updated_at
   FROM knowledge_projection_checkpoints
   WHERE projector_name = 'knowledge-loop-projector';
   ```
3. Snapshot row counts per table for post-check comparison.
   ```sql
   SELECT 'knowledge_loop_entries' AS table, COUNT(*) FROM knowledge_loop_entries
   UNION ALL SELECT 'knowledge_loop_session_state', COUNT(*) FROM knowledge_loop_session_state
   UNION ALL SELECT 'knowledge_loop_surfaces', COUNT(*) FROM knowledge_loop_surfaces;
   ```

## Procedure

**Rule (invariant 18 of the plan): the projection tables are disposable; the dedupe table is NOT.** Reproject TRUNCATEs the three projection tables only.

1. Drain in-flight projector work (wait for current scheduler tick to finish). If running under the long-lived scheduler, it is safe to simply stop the batch midway — the next tick will pick up from the last committed checkpoint.
2. Inside a single transaction:
   ```sql
   BEGIN;
   TRUNCATE knowledge_loop_entries;
   TRUNCATE knowledge_loop_session_state;
   TRUNCATE knowledge_loop_surfaces;
   UPDATE knowledge_projection_checkpoints
   SET last_event_seq = 0, updated_at = NOW()
   WHERE projector_name = 'knowledge-loop-projector';
   COMMIT;
   ```
   **Do NOT truncate `knowledge_loop_transition_dedupes`.** It is an ingest-side idempotency barrier, not a projection; TRUNCATEing it would open a window for duplicate event append on client retry.
3. Let the projector catch up naturally (scheduler tick). To force-accelerate, trigger the job directly:
   ```bash
   # alt-backend container
   curl -X POST http://localhost:9000/internal/jobs/knowledge-loop-projector/run
   ```
4. Monitor checkpoint progress:
   ```sql
   SELECT last_event_seq, updated_at
   FROM knowledge_projection_checkpoints
   WHERE projector_name = 'knowledge-loop-projector';
   ```

## Post-check

1. Checkpoint equals max event seq.
   ```sql
   SELECT
     (SELECT MAX(event_seq) FROM knowledge_events) AS max_seq,
     (SELECT last_event_seq FROM knowledge_projection_checkpoints
      WHERE projector_name = 'knowledge-loop-projector') AS projected_seq;
   -- expect max_seq = projected_seq
   ```
2. Integrity check: `dangling_supersede_refs` view must be empty.
   ```sql
   SELECT COUNT(*) FROM dangling_supersede_refs;
   -- expect 0
   ```
3. Row count within 1-5% of the pre-snapshot (if the projection rules have not changed).

### Cron heartbeat post-check (Pillar 3 — 2026-05-26)

A reproject only resets the `knowledge-loop-projector` row; the
`surface_planner_v2` checkpoint is untouched, and its `updated_at` may stay
frozen on a quiet event log even though the cron is healthy. With the
cron-side heartbeat the row is now bumped every tick regardless of emit —
post-reproject the SLO query below should converge inside one tick interval
(default 60s):

```sql
SELECT projector_name,
       last_event_seq,
       EXTRACT(EPOCH FROM (now() - updated_at))::int AS lag_seconds
FROM knowledge_projection_checkpoints
WHERE projector_name IN ('knowledge-loop-projector', 'surface_planner_v2', 'knowledge-home-projector')
ORDER BY projector_name;
-- expect every projector's lag_seconds <= 2 × tick interval within 5 minutes.
```

A `surface_planner_v2` row whose `lag_seconds` keeps climbing past the tick
interval after the heartbeat fix is shipped means the cron itself is not
running — check `docker compose logs alt-knowledge-sovereign-1 | grep
surface_planner.batch_complete` before assuming a projector hang.

### v2 Surface Planner post-check (Knowledge Loop Completion Phase 1)

Run after a v2 cutover (Wave 4-A augur emit + Wave 4-B recap emit are both
flowing) to confirm `EventLogSurfaceScoreResolver` actually receives
non-zero evidence. A v2 reproject that silently produces all-zero
`surface_score_inputs` means the upstream signal isn't reaching the event
log — investigate before declaring the cutover successful.

4. v2 / v1 placement breakdown.
   ```sql
   SELECT surface_planner_version, surface_bucket, COUNT(*)
   FROM knowledge_loop_entries
   GROUP BY 1, 2
   ORDER BY 1, 2;
   ```
   Expect a non-zero count for `surface_planner_version = 2` once Wave 4-A
   and Wave 4-B are emitting and the resolver has events to score against.
   A 100% v1 result here means the resolver returned `SurfaceScoreInputs{}`
   (zero-value fallback) for every entry — the signal is missing.

5. Evidence non-zero rate per v2 row.
   ```sql
   SELECT
     COUNT(*) FILTER (WHERE surface_score_inputs IS NULL) AS null_inputs,
     COUNT(*) FILTER (WHERE (surface_score_inputs->>'topic_overlap_count')::int > 0) AS topic_overlap_present,
     COUNT(*) FILTER (WHERE (surface_score_inputs->>'has_augur_link')::bool) AS augur_link_present,
     COUNT(*) FILTER (WHERE (surface_score_inputs->>'recap_cluster_momentum')::numeric > 0) AS recap_momentum_present,
     COUNT(*) FILTER (WHERE continue_context IS NULL) AS null_continue_context,
     COUNT(*) FILTER (WHERE change_summary IS NULL) AS null_change_summary,
     COUNT(*) FILTER (WHERE source_observed_at IS NULL) AS null_source_observed_at
   FROM knowledge_loop_entries
   WHERE surface_planner_version = 2;
   ```
   Record these values in the cutover ticket. They are the rollout-visible
   evidence that the Phase 1 emit chain is actually producing signal.

6. Recap target presence in `act_targets[]`.
   ```sql
   SELECT COUNT(*) AS recap_targeted_entries
   FROM knowledge_loop_entries, jsonb_array_elements(act_targets) t
   WHERE t->>'target_type' = 'recap';
   ```
   Expect a non-zero count once recap-worker's persist-stage emit (Phase 1
   §2) is actually running and has produced at least one
   `recap.topic_snapshotted.v1` event whose `top_terms` overlap a Loop
   entry's tags. Zero here while §5 shows a `topic_overlap_present > 0` is
   a smell: the snapshot id field probably failed UUID validation in the
   resolver — inspect a sample with
   `SELECT recap_topic_snapshot_id FROM ...` plus the resolver log.

### v2 Surface Planner post-check (Knowledge Loop Completion Phase 2)

Phase 2 introduced semantic Decide / Act feedback so user actions
(`knowledge_loop.acted.v1` events with `acted_intent` + `target_type` +
`continue_flag`) round-trip back to Loop. Reproject post-checks below verify
that the projector reconstructs `continue_context.recent_action_labels` and
the `RecentContinueActionCount` Continue signal purely from the event log.

7. Bound assertion on `recent_action_labels` (must never exceed 5).
   ```sql
   SELECT COUNT(*) FROM knowledge_loop_entries
   WHERE continue_context IS NOT NULL
     AND jsonb_array_length(continue_context->'recent_action_labels') > 5;
   -- expect 0
   ```
   A non-zero count means the projector wrote an unbounded list — investigate
   `buildContinueContextFromActed` and the bound constant
   (`recentActionLabelsBound`).

8. Semantic action label coverage.
   ```sql
   SELECT
     COUNT(*) FILTER (WHERE continue_context->'recent_action_labels' ? 'opened')   AS opened,
     COUNT(*) FILTER (WHERE continue_context->'recent_action_labels' ? 'asked')    AS asked,
     COUNT(*) FILTER (WHERE continue_context->'recent_action_labels' ? 'saved')    AS saved,
     COUNT(*) FILTER (WHERE continue_context->'recent_action_labels' ? 'compared') AS compared,
     COUNT(*) FILTER (WHERE continue_context->'recent_action_labels' ? 'revisited')AS revisited,
     COUNT(*) FILTER (WHERE continue_context->'recent_action_labels' ? 'snoozed')  AS snoozed
   FROM knowledge_loop_entries;
   ```
   Once Phase 2 has been live for a day, expect non-zero counts across `opened`
   / `asked` / `revisited` (the Continue intents) and at least some `saved` /
   `compared` / `snoozed`. Zero in every column means the frontend isn't
   emitting semantic metadata — check the `/loop/transition` request body in
   the BFF logs.

9. Recap target preservation across reproject.
   ```sql
   -- Run before TRUNCATE:
   CREATE TEMP TABLE reproject_recap_pre AS
   SELECT user_id, entry_key,
          (SELECT COUNT(*) FROM jsonb_array_elements(act_targets) t
           WHERE t->>'target_type' = 'recap') AS recap_targets
   FROM knowledge_loop_entries
   WHERE EXISTS (SELECT 1 FROM jsonb_array_elements(act_targets) t
                 WHERE t->>'target_type' = 'recap');
   -- TRUNCATE + reproject ...
   -- Then assert reproduction:
   SELECT COUNT(*) FROM reproject_recap_pre p
   LEFT JOIN knowledge_loop_entries e
     ON p.user_id = e.user_id AND p.entry_key = e.entry_key
   WHERE e.entry_key IS NULL
      OR (SELECT COUNT(*) FROM jsonb_array_elements(e.act_targets) t
          WHERE t->>'target_type' = 'recap') <> p.recap_targets;
   -- expect 0 (every entry that had a recap target before still has one).
   ```

### Deterministic-replay verification

To confirm the reproject is truly idempotent (a Phase 1 §4 acceptance
criterion), run the full TRUNCATE + replay procedure twice in a row and
diff the resulting projections row-by-row. The outputs must be byte-level
identical for `surface_score_inputs`, `surface_bucket`, `act_targets`,
`why_text`, `why_evidence_refs`, and `change_summary`. Any drift here is a
non-deterministic resolver/projector bug — open a postmortem before
shipping.

```sql
-- After the first reproject, snapshot to a temp table:
CREATE TEMP TABLE reproject_pass1 AS
SELECT entry_key, surface_planner_version, surface_bucket,
       surface_score_inputs::text AS si_text,
       act_targets::text          AS at_text,
       why_text, why_kind,
       change_summary::text       AS cs_text
FROM knowledge_loop_entries
ORDER BY entry_key;

-- Run the TRUNCATE + replay procedure a second time, then:
SELECT COUNT(*) AS divergent_rows
FROM (
  SELECT entry_key, surface_planner_version, surface_bucket,
         surface_score_inputs::text AS si_text,
         act_targets::text          AS at_text,
         why_text, why_kind,
         change_summary::text       AS cs_text
  FROM knowledge_loop_entries
  ORDER BY entry_key
) AS pass2
FULL OUTER JOIN reproject_pass1 USING (entry_key)
WHERE (pass2.surface_bucket, pass2.si_text, pass2.at_text, pass2.why_text,
       pass2.why_kind, pass2.cs_text)
   IS DISTINCT FROM
      (reproject_pass1.surface_bucket, reproject_pass1.si_text,
       reproject_pass1.at_text, reproject_pass1.why_text,
       reproject_pass1.why_kind, reproject_pass1.cs_text);
-- expect 0
```

## Troubleshooting

- **`dangling_supersede_refs` non-empty**: a supersede target is missing from `knowledge_loop_entries`. Check whether the referenced entry was filtered out by the projector (e.g. a `user_id IS NULL` event). Inspect via:
  ```sql
  SELECT * FROM dangling_supersede_refs LIMIT 20;
  ```
  Fix forward by emitting the corrective supersede event; do not backfill directly.
- **Checkpoint stuck**: inspect the projector log for `knowledge_loop_projector: skip event` lines and decide whether to fix-forward with a new event or pin the bad event (out of scope for Loop; it stays in `knowledge_events`).

## WhyMappingVersion history

Each bump triggers a full reproject per the Pre-flight + Procedure above, so existing `knowledge_loop_entries` rows pick up the new `why_text` / `why_evidence_refs` bindings emitted by the projector.

| version | date       | ADR          | what changed |
|---------|------------|--------------|--------------|
| 1       | 2026-04-23 | [[000831]]   | initial mapping table (fixed-string rationales via `shortEventWhy`) |
| 2       | 2026-04-24 | [[000840]]   | `EnrichWhyFromEvent` replaces fixed strings with structured evidence_refs derived from event payload (summary_version_id / tag_set_version_id / article_id / conversation_id / open event_id) |
| 3       | 2026-04-26 | [[000844]]   | why_text rewritten from placeholder strings to substantive narratives; stage-appropriate seedDecisionOptions replaces the previous Source/Observe-only block |
| 4       | 2026-04-26 | [[000844]]   | projector ownership moved to knowledge-sovereign; runtime behavior unchanged from v3 but the bump signals operators that the projection is now driven from this service rather than alt-backend's job runner |
| 5       | 2026-04-26 | [[000846]]   | `SummaryNarrativeBackfilled` event type added so historic entries whose original SummaryVersionCreated event lacked article_title can be patched with a real narrative; optional full reproject post-backfill verifies replay convergence |
| 6       | 2026-04-26 | [[000853]]   | EventLogSurfaceScoreResolver wired via WithScoreResolver; bucket placement now consults v2 evidence (recap topic overlap / augur link / open interaction / version drift) bounded to event.occurred_at - 7d. F-001 enforcement at the resolver seam. canonical contract §6.4.1 / §11 |
| 7       | 2026-04-27 | (this commit) | Surface Planner v2 signal expansion (fb.md §B-2). New SurfaceScoreInputs fields: StalenessScore (pureStalenessBucket of event.OccurredAt - source_observed_at), ContradictionCount, QuestionContinuationScore, RecapClusterMomentum, EvidenceDensity, ReportWorthinessScore. decideBucketV2 priority tightened: ContradictionCount joins VersionDriftCount in CHANGED; QuestionContinuationScore joins HasAugurLink/HasOpenInteraction in CONTINUE; RecapClusterMomentum joins TopicOverlap/TagOverlap in NOW; StalenessScore ≥ 2 elevates REVIEW so it becomes a deliberate re-evaluation queue. Projector also seeds act_targets[] with a Recap target when SurfaceScoreInputs.RecapTopicSnapshotID is non-empty. Run the standard Procedure to backfill v7 placements + new act_targets. |
| 8       | 2026-05-01 | —             | SurfacePlanRecomputed projector branch added. The system-only replan event patches planner-owned entry placement columns (surface_bucket, render_depth_hint, loop_priority, surface_planner_version, surface_score_inputs) without touching why / lifecycle / freshness fields, then recomputes the four surfaces. Bump signals operators to include the new branch in replay validation. |
| 9       | 2026-05-09 | —             | Phase 3 (knowledge-loop-completion-03-review-why-quality). why_override priority reordered to canonical contract §11: change > unfinished_continue > topic_affinity > tag_trending > recall > source. RECALL was previously checked before topic / tag overlap; v9 makes it the residual evidence kind so a single prior open does not crowd out an active recap-cluster or tag-stream connection. Also: KnowledgeLoopReviewed projector branch split into recheck / archive / mark_reviewed lifecycle outcomes via PatchKnowledgeLoopEntryReviewLifecycle — mark_reviewed keeps the entry visible in Review (was hidden under v8); archive keeps the existing hide-from-read-path semantics. Run the standard Procedure; post-check that visible Review rows now include those marked reviewed and exclude those archived. |
| 10      | 2026-05-23 | [[000908]]    | ADR-000908 §Δ1 ActOutcomeSignal lands as a bucket driver. EventLogSurfaceScoreResolver aggregates `knowledge_loop.act_outcome.v1` events on the entry inside the 7d window (engaged=+1, deep_engagement=+2, accepted_change=+1, stale_save=-1, no_engagement=-2) and decideBucketV2 demotes Now/Continue placements to Review when the cumulative signal is ≤ -2. CHANGED still outranks the demotion so version drift is never silently hidden. Run the standard Procedure to backfill v10 placements. |
| 11      | 2026-05-25 | [[000908]]    | ADR-000908 §Δ4 WhyPayload v2 producer wiring lands. The projector populates three additive WhyPayload fields on every entry: `counter_evidence_refs` (≤4, supersede branch only), `confidence_ladder` (speculation/pattern/evidence/verified — pure function of WhyKind), `what_would_change_my_mind` (1..256 char falsifier sentence per WhyKind). Sovereign proto and `knowledge_loop_entries` schema gain three additive columns via migration `00021_add_why_v2_columns.sql`; alt-backend BFF passes the new fields through to the public `alt.knowledge.loop.v1.WhyPayload` wire type. Run the standard Procedure to backfill v11; post-check that `curl /v1/knowledge-loop \| jq '.entries[].whyPrimary.confidenceLadder'` is non-null on ≥80% of visible rows (the rest are kinds with no claim). |
| 12      | 2026-05-25 | [[000907]]    | ADR-000907 epistemic-change review driver lands. `decideReviewReason` (pure function of SurfaceScoreInputs) populates a new `review_reason` column on every entry via migration `00022_add_review_reason.sql`. Priority: version_drift > contradiction > unfinished_thread > staleness > none. Sovereign + alt-backend protos gain `ReviewReason` enum + `KnowledgeLoopEntry.review_reason` (field 30 on the public proto, 29 on sovereign). Run the standard Procedure to backfill v12; post-check `curl /v1/knowledge-loop \| jq '.entries[].reviewReason' \| sort \| uniq -c` shows a mix of `version_drift` / `contradiction` / `unfinished_thread` / `staleness` / `none` rather than all `none`. |
| 13      | 2026-05-25 | [[000913]]    | ADR-000913 §D-10 persist-stage calibrated uncertainty lands. SurfaceScoreInputs gains `ConfidenceLadder`; `decideBucketV2` demotes NOW/CONTINUE to REVIEW when `ConfidenceLadder == SPECULATION` (same priority as `ActOutcomeSignal ≤ -2`, CHANGED still wins). The recap-worker publishes `persist_stage_confidence_ladder` on TopicSnapshotted / SurfacePlanRecomputed payloads; the projector reads it via `parseSurfaceScoreInputs`. Bump triggers a full reproject so entries projected before the recap-worker started emitting the ladder pick up the default (UNSPECIFIED → no demotion). Post-check: `curl /v1/knowledge-loop \| jq '.entries[].whyPrimary.confidenceLadder' \| sort \| uniq -c` shows SPECULATION ladder rows once recap-worker has fired against a low-density cluster. |

## SurfacePlannerVersion history

Each bump triggers a full reproject so that legacy entries pick up the new bucket placement deterministically. The bumped value is recorded on each row in `KnowledgeLoopEntry.surface_planner_version` (proto field 23).

| version | date       | ADR          | what changed |
|---------|------------|--------------|--------------|
| `v1`    | 2026-04-23 | [[000831]]   | event-type → bucket fixed mapping (`SummaryVersionCreated` → Now, `HomeItemOpened` → Continue, `HomeItemSuperseded` → Changed, `HomeItemDismissed` → Review) |
| `v2`    | 2026-04-27 | [[000856]]   | knowledge-state-based scoring via `decideBucketV2(SurfaceScoreInputs) → SurfaceBucket`. Inputs are immutable evidence drawn from `event.occurred_at - 7d` windows on versioned tables only; `surface_planner_version` and `surface_score_inputs` are written through the Sovereign projection path |

### v2 cutover checklist (in addition to the Procedure above)

1. Confirm the upstream snapshot events are flowing: `RecapTopicSnapshotted` (recap-worker) and `AugurConversationLinked` (augur). The v2 planner is starved without them and falls back to v1 placement.
2. Ensure the new evidence columns / tables required by v2 inputs exist (see Atlas migration history).
3. Drain in-flight projector work and run the Procedure transaction.
4. Verify the post-check `SELECT DISTINCT surface_planner_version` returns exactly `v2` once the projector has caught up.

- **`projected_at` in API response**: a serious bug. The canonical contract forbids it. Treat as a security incident; see ADR-000831 §12 and the `TestProjectedAtNotSerialized` test.
