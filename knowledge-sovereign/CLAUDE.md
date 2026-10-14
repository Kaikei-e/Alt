# knowledge-sovereign

Durable knowledge state owner (event log, Knowledge Home projector, Recall projector, and Knowledge Trail projector/planner).

## Core Responsibilities

- **Event Log**: Append-only `knowledge_events` and `knowledge_user_events` with idempotency via `knowledge_event_dedupes`.
- **Knowledge Home Projector**: Projects `knowledge_home_items` and `today_digest_view` for the active `projection_version`, skipping empty-title items and computing time-decayed base scores with tag/summary boosts.
- **Recall Projector**: Projects `recall_candidate_view` with deduplicated `reason_json` codes per `(user_id, item_key)` and clears dismissed items (`ClearNextReviewAt`).
- **Knowledge Trail Projector & Planner**:
  - Records `knowledge_trail_footprints` only for explicit engagement verbs (`opened`, `saved`, `asked`, `noted`, `revisited`).
  - Proposes `cluster` branches only when recent engagement anchors share at least 2 normalized tags with candidate articles.

## Commands

```bash
cd knowledge-sovereign/app
go test ./...
```
