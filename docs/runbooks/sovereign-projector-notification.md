# Sovereign Projector Notification — Troubleshooting & Design Record

## Background

When Knowledge Sovereign is extracted as an independent service with its own database (database-per-service pattern), the knowledge projector in the backend service can no longer use PostgreSQL `LISTEN/NOTIFY` directly on the events table. The events table exists only in the sovereign database, and direct cross-service database connections violate the service boundary.

## Architecture

```
backend ──Connect-RPC stream──▶ sovereign ──pgx LISTEN──▶ sovereign-db
                                     │
                           knowledge_events INSERT
                                     │
                           NOTIFY trigger fires
                                     │
                           sovereign pushes to stream
                                     │
backend projector wakes ◀────────────┘
```

- **sovereign** owns the `knowledge_events` table and the `LISTEN` connection.
- **backend** receives notifications through a Connect-RPC server-streaming RPC (`WatchProjectorEvents`).
- The projector runner's existing poll-timeout loop is preserved; the streaming listener plugs into the same `KnowledgeProjectorListener` interface.

## Key Design Decisions

### Why streaming, not polling-only

| Approach | Latency | Efficiency | Complexity |
|----------|---------|------------|------------|
| Polling (5s) | 0–5s | ~98% wasted polls | Low |
| Server streaming | ~0s | Push-only | Medium |
| Webhook | ~0s | Push-only | Medium (needs endpoint) |

Server streaming was chosen because:
1. Connect-RPC is already in use between the services — no new infrastructure.
2. Sovereign owns the database, so it has the right to `LISTEN`.
3. The backend does not need a publicly routable callback endpoint.

References:
- [Webhooks vs Polling vs Streaming](https://medium.com/@stoic.engineer/webhooks-vs-polling-vs-streaming-choosing-the-right-event-communication-pattern-c7eab10c7191)
- [gRPC Long-lived Streaming](https://dev.bitolog.com/grpc-long-lived-streaming/)
- [gRPC Keepalive](https://grpc.io/docs/guides/keepalive/)

### Channel-based pump pattern

The streaming client uses a **single pump goroutine** that reads from the gRPC/Connect stream and writes to a Go channel. `WaitForNotification(ctx)` selects on that channel and the caller's context, avoiding goroutine leaks.

```
pump goroutine:          for stream.Receive() { notify <- struct{}{} }
WaitForNotification:     select { case <-ctx.Done(): ... case <-notify: ... }
```

This composes correctly with the projector runner's poll-timeout model without modifying the runner.

## Known Pitfalls

### 1. HTTP client timeout kills streaming connections

**Symptom**: Streaming connection disconnects after exactly N seconds, matching the HTTP client's `Timeout` field.

**Cause**: The unary RPC client typically has a global `http.Client.Timeout` (e.g., 30s). If the same client is reused for streaming, the timeout kills the long-lived connection.

**Fix**: Create a **separate `http.Client` without `Timeout`** for streaming RPCs.

```go
// Unary client — global timeout is fine
unaryHTTP := &http.Client{Timeout: 30 * time.Second}

// Streaming client — no global timeout
streamHTTP := &http.Client{
    Transport: &http.Transport{IdleConnTimeout: 90 * time.Second},
    // No Timeout field — streaming connections are long-lived.
}
```

### 2. Heartbeat interval must be shorter than poll interval

**Symptom**: The projector runner calls `WaitForNotification` with a context timeout equal to its poll interval (e.g., 5s). If the server heartbeat interval is longer (e.g., 30s), the client's context always expires before a heartbeat arrives, and the runner treats the stream as failed.

**Fix**: Set the server-side heartbeat interval shorter than the client's poll interval. Currently: heartbeat = 3s, poll interval = 5s.

### 3. LISTEN/NOTIFY requires a NOTIFY trigger on the events table

The sovereign database must have a trigger that fires `pg_notify` on INSERT into `knowledge_events`:

```sql
CREATE OR REPLACE FUNCTION notify_knowledge_projector() RETURNS trigger AS $$
BEGIN
  PERFORM pg_notify('knowledge_projector', NEW.event_seq::text);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_knowledge_events_notify
  AFTER INSERT ON knowledge_events
  FOR EACH ROW
  EXECUTE FUNCTION notify_knowledge_projector();
```

If this trigger is missing, the streaming RPC will only send heartbeats and never real notifications.

### 4. Projector stalls silently after service extraction

**Symptom**: After extracting a service with its own database, the projector stops processing events. No error logs appear — the projector simply never wakes up.

**Cause**: The `LISTEN` connection still points to the old shared database where the events table no longer exists (or no longer receives INSERTs). The listener connects successfully (the old database is still running), but notifications never arrive.

**Detection**: Check for `"processing knowledge events"` log entries. If absent for several minutes while events exist in the sovereign database, the projector is stalled.

## Diagnostic Commands

```bash
# Check if projector listener is connected
docker logs <backend-container> | grep "sovereign projector listener"

# Check if projector is processing events
docker logs <backend-container> | grep "processing knowledge events"

# Check sovereign streaming connections
docker logs <sovereign-container> | grep "WatchProjectorEvents"

# Verify NOTIFY trigger exists in sovereign DB
docker exec <sovereign-db-container> psql -U <user> -d <db> -c \
  "SELECT tgname FROM pg_trigger WHERE tgname = 'trg_knowledge_events_notify';"

# Manually test NOTIFY
docker exec <sovereign-db-container> psql -U <user> -d <db> -c \
  "NOTIFY knowledge_projector, 'test';"
# Then check backend logs for projector activity.
```

## Recovery

If the projector is stalled:

1. **Restart the backend service** — the streaming listener reconnects automatically.
2. If the listener fails to connect, check sovereign health: `curl http://<sovereign>:9500/health`.
3. If sovereign is healthy but streaming fails, check firewall/network between services.
4. As a fallback, the projector runner automatically falls back to polling if the listener factory returns an error.
