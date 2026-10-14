# knowledge-sovereign

Durable knowledge state owner (event log, Knowledge Home projector, and Knowledge Trail projector/planner).

## Core Responsibilities

- **Event Log**: Append-only `knowledge_events` and `knowledge_user_events` with idempotency via `knowledge_event_dedupes`.
- **Knowledge Home Projector & Read Path**:
  - `usecase/knowledge_home_projector` folds events into `knowledge_home_items`, `today_digest_view`, and `recall_candidate_view` for the active `projection_version`.
  - Stores a flat, time-invariant `baseQualityScore` (`0.5`) with summary (`0.8`) and tag (`0.7`) floor boosts (`score_op: "max"`) and open suppression (`0.1`, `score_op: "set"`).
  - Time-decay ranking (`homeItemRankScoreSQL`) and empty-title filtering (`COALESCE(khi.title, '') <> ''`) happen at read time in `GetKnowledgeHomeItems` (`driver/sovereign_db/read_projections.go`), not in the projector.
- **Recall Candidates**: `UpsertRecallCandidate` (`driver/sovereign_db/mutation_recall_candidate.go`) deduplicates `reason_json` entries by `(type, description, source_item_key)` per `(user_id, item_key)` and `DismissRecallCandidate` soft-deletes dismissed items by setting `dismissed_at`.
- **Knowledge Trail Projector & Planner**:
  - `EngagementVerbs` are `read`, `asked`, and `listened` (`driver/sovereign_db/read_trail.go`). Only these verbs appear on the trail spine, count toward path wear, or anchor branches (`dismissed` footprints are excluded).
  - `DeriveTrailClusterCandidates` (`driver/sovereign_db/read_trail_candidates.go`) normalizes tags and excludes junk tags (`usecase/tagclean`) in SQL before ranking, selecting the anchor with the largest clean tag overlap (ties broken by most recent engagement).
  - `usecase/trail_planner` requires at least 1 clean shared tag (`len(c.SharedTags) >= 1` after `tagclean.CleanDisplay`) to propose a `cluster` branch (`confidence: "plausible"` for 1 clean shared tag, `"corroborated"` for 2 or more).

## Commands

```bash
cd knowledge-sovereign/app
go test ./...
```
