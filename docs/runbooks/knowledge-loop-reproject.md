---
title: Knowledge Loop Reproject Runbook
date: 2026-04-23
status: proposed
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
- **`projected_at` in API response**: a serious bug. The canonical contract forbids it. Treat as a security incident; see ADR-000831 §12 and the `TestProjectedAtNotSerialized` test.
