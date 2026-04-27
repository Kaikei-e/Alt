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
