---
title: "Knowledge Home Stream Disconnect Surge"
date: 2026-03-18
tags:
  - runbook
  - knowledge-home
  - alt-backend
  - streaming
---

# Knowledge Home Stream Disconnect Surge

Runbook for investigating server-side stream disconnects affecting real-time Knowledge Home updates.

Related: [[000418]]

## Alerts

| Alert | Severity | Condition |
|-------|----------|-----------|
| `KnowledgeHomeStreamDisconnectSurge` | page | > 80% disconnect rate for 5m |

## Symptoms

- `KnowledgeHomeStreamDisconnectSurge` alert fires.
- Users lose real-time Knowledge Home updates; items only refresh on page reload.
- Client-side logs show repeated SSE/WebSocket reconnection attempts.
  (`alt_home_stream_reconnects_total` is declared but never incremented
  server-side, so this shows up only in browser logs, not `/metrics`.)
- After 3 consecutive failures, clients fall back to unary polling.

## Investigation

### 1. Check alt-backend resource usage

```bash
# Memory and CPU via cAdvisor or docker stats
docker stats alt-alt-backend-1 --no-stream

# Check for OOM kills
docker inspect alt-alt-backend-1 --format='{{.State.OOMKilled}}'

# Goroutine count. pprof (internal/profiling) listens on its own :6060, not
# the REST port :9000 -- and only when PPROF_ENABLED=true (unset by default
# in compose, so off unless a .env override enables it). :6060 is not
# published to the host either, so this needs a debug container on
# alt_alt-network rather than a host-side curl:
docker run --rm --network alt_alt-network busybox:1.37 \
  wget -qO- http://alt-backend:6060/debug/pprof/goroutine?debug=1 | head -5
```

High goroutine count (> 10,000) or memory above 80% of limit indicates a leak.

### 2. Check alt-backend's own database access

`cmd/backend` holds no direct Postgres connection at all (enforced by
`di/import_boundary_test.go`) -- it reads through `alt-data-hub`'s
Connect-RPC surface, and Knowledge Home data specifically comes from
`knowledge-sovereign`, which dials `knowledge-sovereign-db` directly with no
pooler in front of it. PgBouncer only fronts `alt-db`/Kratos and is not on
this path, so PgBouncer pool exhaustion cannot itself explain a Knowledge
Home stream disconnect. If alt-data-hub's own DB access is suspect:

```bash
docker compose -f compose/compose.yaml -p alt exec pgbouncer sh -lc "psql -p 6432 -U alt_db_user pgbouncer -c 'SHOW POOLS;'"
docker compose -f compose/compose.yaml -p alt exec pgbouncer sh -lc "psql -p 6432 -U alt_db_user pgbouncer -c 'SHOW CLIENTS;'"
```

### 3. Check LISTEN/NOTIFY health

The stream is fed by a Connect-RPC call (`WatchProjectorEvents`) from
alt-backend to `knowledge-sovereign`, which holds a `LISTEN knowledge_projector`
connection on `knowledge-sovereign-db` (not `alt-db`, and not a channel
literally named `knowledge_home_updates`):

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -c \
  "SELECT * FROM pg_listening_channels();"
```

If `knowledge_projector` is not listed, `knowledge-sovereign`'s listener has
disconnected -- check its logs for the `WatchProjectorEvents` stream and for
reconnection to `knowledge-sovereign-db`.

### 4. Check network and edge proxy

The edge is `plecto-proxy` (`compose/core.yaml`); there is no `nginx` service and no
nginx-exporter. Its admin listener serves `/metrics`, `/healthz` and `/readyz` on
`0.0.0.0:8080` inside the container (`plecto/manifest.toml`, `[observability]
admin_addr`), published to the host as `127.0.0.1:8080` only.

Never pipe these `curl`s straight into `grep`: that hides curl's exit code *and*, with
`-s`, its error message, so an unreachable proxy is indistinguishable from "no matching
metric". Capture to a file first, then filter. Always pass `-f` as well: without it a
non-2xx admin response (wrong path, renamed endpoint, auth added) still writes an empty
file and exits 0, which reads exactly like "collected, nothing matched".

```bash
# Edge readiness. -sS keeps the error on stderr; -w prints the status code.
curl -sS --noproxy '*' -w '\nHTTP %{http_code}\n' http://127.0.0.1:8080/readyz

# Edge metrics. -f turns a non-2xx into a non-zero exit so the || branch fires.
curl -fsS --noproxy '*' http://127.0.0.1:8080/metrics > /tmp/plecto-metrics.txt \
  || echo "UNREACHABLE: plecto-proxy admin :8080 -- edge signal NOT collected"
grep -iE "connection|upstream|request" /tmp/plecto-metrics.txt

# Edge logs. No 2>&1: "no such service" goes to stderr and must stay visible.
docker compose -f compose/compose.yaml -p alt logs plecto-proxy --since=15m \
  > /tmp/plecto-logs.txt \
  || echo "FAILED to collect plecto-proxy logs -- edge signal NOT collected"
grep -iE "upstream|timeout|502|504|disconnect" /tmp/plecto-logs.txt
```

If Prometheus is running (`compose/observability.yaml`), the same metrics are scraped
under `job="plecto-proxy"` (`observability/prometheus/prometheus.yml`):

```bash
curl -sS --noproxy '*' -w '\nHTTP %{http_code}\n' \
  'http://127.0.0.1:9090/api/v1/query?query=up%7Bjob%3D%22plecto-proxy%22%7D'
```

### 5. Check disconnect reasons

```bash
docker compose -f compose/compose.yaml -p alt logs alt-backend --since=15m 2>&1 | grep "stream_disconnect\|reason="
```

Common reasons:
- `context_canceled` -- normal client navigation away.
- `write_timeout` -- alt-backend could not send update within deadline.
- `listener_error` -- PostgreSQL LISTEN connection dropped.
- `oom_pressure` -- memory pressure forced connection cleanup.

## Resolution

### Memory leak in alt-backend

1. Confirm with `docker stats alt-alt-backend-1` showing climbing memory.
2. Restart alt-backend:
   ```bash
   docker compose -f compose/compose.yaml -p alt restart alt-backend
   ```
3. Monitor goroutine count after restart. If it climbs again, file a bug with pprof output (see the :6060 / PPROF_ENABLED caveat in Investigation step 1):
   ```bash
   docker run --rm --network alt_alt-network -v /tmp:/out busybox:1.37 \
     wget -qO /out/heap.prof http://alt-backend:6060/debug/pprof/heap
   go tool pprof -top /tmp/heap.prof
   ```

### knowledge-sovereign-db connection exhaustion

`knowledge-sovereign` holds its own direct (unpooled) connection to
`knowledge-sovereign-db` for the `WatchProjectorEvents` LISTEN -- there is no
PgBouncer on this path, so "PgBouncer exhaustion" is not a cause of Knowledge
Home stream disconnects.

1. Check `knowledge-sovereign-db`'s own connection count and `max_connections` (Investigation step 2's `pg_stat_activity` query).
2. If `knowledge-sovereign-db` itself is unresponsive:
   ```bash
   docker compose -f compose/compose.yaml -p alt restart knowledge-sovereign-db
   ```

### LISTEN/NOTIFY channel disconnected

1. `knowledge-sovereign`'s listener auto-reconnects, but if it is stuck:
   ```bash
   docker compose -f compose/compose.yaml -p alt restart knowledge-sovereign
   ```
2. After restart, verify the channel is re-registered (see investigation step 3), then confirm alt-backend's `WatchProjectorEvents` stream client has reconnected (alt-backend logs).

### Edge proxy terminating long-lived streams

1. Knowledge Home's Connect-RPC stream reaches the browser through the
   `path_prefix = "/api/v2/alt.knowledge_home.v1."` route in `plecto/manifest.toml`,
   whose upstream is `alt-frontend-sv` (the SvelteKit SSR proxy), not alt-backend
   directly. Confirm that route is still enumerated -- a streaming prefix that is not
   listed falls through to the generic `/api/` route.
2. `plecto/manifest.toml` declares no read/idle timeout knob, so there is no
   `proxy_read_timeout` equivalent to raise. If the edge is cutting streams, the
   evidence has to come from the plecto-proxy logs and metrics in investigation step 4;
   file it against PlectoProxy rather than editing an nginx config that no longer
   exists.

## Verification

After resolution:
- `alt_home_stream_disconnects_total` rate (excluding `client_close`) drops below 20% of connections.
- `alt_home_stream_connections_total` rate stabilizes.
- Users confirm real-time updates are working without page reload.
- `docker stats alt-alt-backend-1` shows stable memory usage.
