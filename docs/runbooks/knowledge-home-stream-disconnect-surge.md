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
- `alt_home_stream_reconnects_total` counter is climbing rapidly.
- After 3 consecutive failures, clients fall back to unary polling.

## Investigation

### 1. Check alt-backend resource usage

```bash
# Memory and CPU via cAdvisor or docker stats
docker stats alt-backend --no-stream

# Check for OOM kills
docker inspect alt-backend --format='{{.State.OOMKilled}}'

# Goroutine count (if pprof is enabled)
curl -s http://localhost:9000/debug/pprof/goroutine?debug=1 | head -5
```

High goroutine count (> 10,000) or memory above 80% of limit indicates a leak.

### 2. Check PgBouncer connections

```bash
docker exec alt-pgbouncer sh -lc "psql -p 6432 -U alt_db_user pgbouncer -c 'SHOW POOLS;'"
docker exec alt-pgbouncer sh -lc "psql -p 6432 -U alt_db_user pgbouncer -c 'SHOW CLIENTS;'"
```

Look for:
- `cl_waiting` > 0 -- clients are waiting for connections.
- `sv_active` at limit -- all server connections in use.

### 3. Check LISTEN/NOTIFY health

Knowledge Home streaming relies on PostgreSQL LISTEN/NOTIFY. Verify the notification channel is working:

```bash
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -c \
  \"SELECT * FROM pg_listening_channels();\""
```

If `knowledge_home_updates` is not listed, the listener has disconnected.

### 4. Check network and proxy

```bash
# Nginx connection stats
curl -s http://localhost:9113/metrics | grep nginx_connections

# Check for upstream timeouts in nginx logs
docker compose -f compose/compose.yaml -p alt logs nginx --since=15m 2>&1 | grep -i "upstream\|timeout\|502\|504"
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

1. Confirm with `docker stats alt-backend` showing climbing memory.
2. Restart alt-backend:
   ```bash
   docker compose -f compose/compose.yaml -p alt restart alt-backend
   ```
3. Monitor goroutine count after restart. If it climbs again, file a bug with pprof output:
   ```bash
   curl -o /tmp/heap.prof http://localhost:9000/debug/pprof/heap
   go tool pprof -top /tmp/heap.prof
   ```

### PgBouncer connection exhaustion

1. Check if long-running LISTEN connections are consuming the pool.
2. LISTEN/NOTIFY requires session-level pooling. Verify the alt-backend LISTEN connection uses a dedicated (non-pooled) connection.
3. If PgBouncer is exhausted for query connections:
   ```bash
   docker compose -f compose/compose.yaml -p alt restart alt-pgbouncer
   ```
4. Consider increasing `max_client_conn` or `default_pool_size` in PgBouncer config if the pattern recurs.

### LISTEN/NOTIFY channel disconnected

1. The listener auto-reconnects, but if it is stuck:
   ```bash
   docker compose -f compose/compose.yaml -p alt restart alt-backend
   ```
2. After restart, verify the channel is re-registered (see investigation step 3).

### Nginx proxy timeout

1. If nginx is terminating long-lived connections, check `proxy_read_timeout` in the nginx config for the stream endpoint.
2. The stream endpoint should have a longer timeout than standard API endpoints (recommend >= 300s).

## Verification

After resolution:
- `alt_home_stream_disconnects_total` rate (excluding `client_close`) drops below 20% of connections.
- `alt_home_stream_connections_total` rate stabilizes.
- Users confirm real-time updates are working without page reload.
- `docker stats alt-backend` shows stable memory usage.
