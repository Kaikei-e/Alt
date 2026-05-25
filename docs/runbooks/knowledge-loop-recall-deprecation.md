---
title: Knowledge Home Recall Deprecation Watch
date: 2026-05-25
status: accepted
tags:
  - runbook
  - knowledge-home
  - deprecation
  - recall-rail
aliases:
  - knowledge-loop-recall-deprecation
---

# Knowledge Home Recall Deprecation Watch

ADR: [[000913]]
Canonical contract: [[knowledge-loop-canonical-contract]] §6.4 (Review bucket
absorbs the recall rail under ADR-000913 §D-9)

## Purpose

ADR-000913 §D-9 / planning doc `knowledge-loop-evolution-remaining-tasks.md`
PR 3 lands a `legacy.recall_rail.deprecated` warn log on every invocation of
the three legacy recall endpoints in `alt-backend`:

| RPC | Status |
|---|---|
| `alt.knowledge_home.v1.KnowledgeHomeService/GetRecallRail` | Deprecated, served |
| `alt.knowledge_home.v1.KnowledgeHomeService/TrackRecallAction` | Deprecated, served |
| `alt.knowledge_home.v1.KnowledgeHomeService/StreamRecallRailUpdates` | Deprecated, served |

The endpoints still serve real traffic — the goal of this window is to watch
the deprecation count and confirm it falls to zero before PR 13 removes
them.

## What to watch

The log line is emitted via the `slog.Logger` attached to the
`alt-backend` knowledge_home handler:

```
level=warn msg="legacy.recall_rail.deprecated" rpc="GetRecallRail" user_id=<uuid> ...
```

The `rpc` attribute is one of `GetRecallRail`, `TrackRecallAction`, or
`StreamRecallRailUpdates`. The `user_id` attribute is the authenticated user.

## Grafana / log query

For installations using the Rask log aggregator (ClickHouse `rask_logs.otel_logs`):

```sql
SELECT
  toDate(Timestamp)                          AS day,
  JSONExtractString(Body, 'rpc')             AS rpc,
  count()                                    AS hits,
  uniq(JSONExtractString(Body, 'user_id'))   AS distinct_users
FROM rask_logs.otel_logs
WHERE Timestamp >= now() - INTERVAL 7 DAY
  AND JSONExtractString(Body, 'msg') = 'legacy.recall_rail.deprecated'
GROUP BY day, rpc
ORDER BY day DESC, hits DESC;
```

For plain `loki` / `vector` queries, look for `legacy.recall_rail.deprecated`
in alt-backend's log stream.

## Observation KPI

- **Per-day hit count by `rpc`**: must drop to **0** for 7 consecutive
  release days before PR 13 lands.
- **Distinct user count by `rpc`**: tracks whether the residual traffic is
  one stuck client or many. A long-tail of distinct users probably means a
  cached frontend bundle — wait for the natural rollover before forcing.
- **Stream traffic on `StreamRecallRailUpdates`**: long-lived streams open
  before the deprecation log landed will keep emitting until reconnect.
  Confirm the count drops as expected after `alt-frontend-sv` rollout that
  ships PR 10's single-source migration.

## Cutover plan

1. PR 3 lands (this runbook, deprecation log, test). No behaviour change.
2. PR 10 lands FE single-source migration. Frontend stops calling the
   legacy endpoints.
3. Watch the Grafana panel above. Target: 7 release days with `hits = 0`
   per rpc.
4. PR 13 lands the proto + handler removal. wire compat is preserved via
   `reserved` field numbers on the proto service.

If `hits > 0` after PR 10 ships, **do not** proceed to PR 13. Investigate
the remaining caller (mobile cache, third-party integration, e2e fixture
that wasn't cleaned up, …) and re-cycle the watch.

## Rollback

The deprecation log is informational only — no rollback needed for PR 3.
If the log spam is unexpectedly noisy, downgrade the level to `Info` in
`alt-backend/app/connect/v2/knowledge_home/home_recall.go` before reverting
the entire log.

## Related

- `docs/plan/knowledge-loop-evolution-remaining-tasks.md` — PR 3 / PR 13
  acceptance criteria
- ADR-000913 §D-9 — Heavy-Ranker explainable scoring and the rationale for
  merging the recall rail into GetKnowledgeHome
