# log-seeker — symptom recipes

Pick the section that matches the symptom. All commands are **read-only**. The compose
handle `C` below is `docker compose -f compose/compose.yaml -p alt`. ClickHouse and Postgres
connect recipes (env + secrets) are in [targets.md](targets.md).

## Contents
- Service throwing errors / 5xx
- Slow / high latency
- Container restarting / crash-looping / OOM
- Knowledge Home empty or malformed `why`
- Projector lag / stuck OODA loop
- DB connection exhaustion / pool saturation
- Queue stuck (mq-hub / Redis Streams)
- Trace one request end-to-end
- Recap / Acolyte degradation
- Existing scripts to reuse

## Service throwing errors / 5xx

```bash
$C logs --since=30m --timestamps <svc> | grep -iE 'error|panic|fatal|traceback|exception' | tail -50
```
ClickHouse — errors by service in the last hour, then the latest rows:
```sql
SELECT ServiceName, count() AS n FROM otel_error_logs
WHERE Timestamp > now() - INTERVAL 1 HOUR GROUP BY ServiceName ORDER BY n DESC;

SELECT Timestamp, ServiceName, SeverityText, Body, TraceId FROM otel_error_logs
WHERE Timestamp > now() - INTERVAL 1 HOUR ORDER BY Timestamp DESC LIMIT 50;
```
HTTP 5xx by route (column names may be `status`/`StatusCode` — `SHOW CREATE TABLE otel_http_requests` to confirm):
```sql
SELECT route, status, count() AS n FROM otel_http_requests
WHERE timestamp > now() - INTERVAL 1 HOUR AND status >= 500 GROUP BY route, status ORDER BY n DESC;
```

## Slow / high latency

```sql
-- request latency percentiles by route (last hour)
SELECT route,
       quantile(0.50)(duration_ms) AS p50,
       quantile(0.95)(duration_ms) AS p95,
       quantile(0.99)(duration_ms) AS p99,
       count() AS n
FROM otel_http_requests WHERE timestamp > now() - INTERVAL 1 HOUR
GROUP BY route ORDER BY p99 DESC LIMIT 20;
```
Postgres — slowest statements (needs `pg_stat_statements`; skip if extension absent):
```sql
SELECT calls, round(total_exec_time) AS total_ms, round(mean_exec_time) AS mean_ms, left(query, 120) AS q
FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 15;
```
Reuse:
```bash
bash scripts/analyze-nginx-logs.sh                       # nginx latency / status breakdown
uv run python scripts/analyze_clickhouse_performance.py --hours 6   # platform health snapshot
```

## Container restarting / crash-looping / OOM

```bash
$C ps                                                                # look for Restarting / unhealthy
docker inspect --format '{{.RestartCount}} OOM={{.State.OOMKilled}} exit={{.State.ExitCode}} status={{.State.Status}}' <container_name>
$C logs --tail=200 --timestamps <svc>                                # last words before death
grep -n 'mem_limit' compose/*.yaml | grep -i <svc>                   # is the limit too low?
```
Then ClickHouse `otel_error_logs` / `logs` around the restart timestamp for the trigger.

## Knowledge Home empty or malformed `why`

All against **knowledge-sovereign-db** (`$C exec -T knowledge-sovereign-db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off -c "<sql>"'`):

```sql
SELECT count(*) AS items, max(updated_at) AS newest FROM knowledge_home_items;
SELECT projector_name, last_event_seq, updated_at,
       EXTRACT(EPOCH FROM now() - updated_at)::int AS lag_seconds
FROM knowledge_projection_checkpoints ORDER BY lag_seconds DESC;
SELECT count(*) FROM knowledge_events;          -- and `\d knowledge_events` to see its columns before tailing it
```
Then escalate to the runbook: `docs/runbooks/knowledge-home-empty-spike.md` (empty),
`docs/runbooks/knowledge-home-malformed-why-spike.md` (corrupted `why`),
`docs/runbooks/knowledge-home-reproject-operations.md` (rebuild). Do **not** run a reproject
yourself — propose it and point at the runbook.

## Projector lag / stuck OODA loop

Against **knowledge-sovereign-db**:

```sql
SELECT projector_name, last_event_seq, updated_at,
       EXTRACT(EPOCH FROM now() - updated_at)::int AS lag_seconds
FROM knowledge_projection_checkpoints ORDER BY lag_seconds DESC;

SELECT status, count(*) FROM knowledge_loop_entries GROUP BY status;   -- run `\d knowledge_loop_entries` for the lease/state columns
SELECT * FROM knowledge_reproject_runs ORDER BY started_at DESC LIMIT 10;
```
Runbook: `docs/runbooks/knowledge-loop-reproject.md`, `docs/runbooks/knowledge-home-reproject-operations.md`.

## DB connection exhaustion / pool saturation

```bash
$C logs --since=30m --timestamps pgbouncer       # pool-full / auth errors land here
# admin console (only if a stats/admin user is provisioned — otherwise "FATAL: not allowed"):
$C exec -T pgbouncer sh -c 'psql -h 127.0.0.1 -p 6432 -U "$DB_USER" pgbouncer -c "SHOW POOLS;" -c "SHOW STATS;" -c "SHOW CLIENTS;"'
bash scripts/pgbouncer_stats.sh
```
```sql
-- on the backing Postgres (db / kratos-db / …)
SELECT state, count(*) FROM pg_stat_activity GROUP BY state;
SHOW max_connections;
SELECT pid, usename, application_name, now() - query_start AS dur, left(query, 120) AS q
FROM pg_stat_activity WHERE state <> 'idle' AND query_start IS NOT NULL ORDER BY dur DESC LIMIT 15;
SELECT * FROM pg_locks WHERE NOT granted;
```

## Queue stuck (mq-hub / Redis Streams)

```bash
$C logs --since=30m --timestamps mq-hub | grep -iE 'error|stuck|retry|deadletter|lag'
$C exec -T redis-streams redis-cli INFO clients
$C exec -T redis-streams redis-cli --scan --pattern '*' | head
$C exec -T redis-streams redis-cli XINFO STREAM <stream-key>
$C exec -T redis-streams redis-cli XLEN <stream-key>
$C exec -T redis-streams redis-cli XPENDING <stream-key> <group>
```
Cross-check the queue-saturation notes (3-layer fix from 2026-02) before recommending a knob change.

## Trace one request end-to-end

1. Find a `trace_id`/`TraceId` — in `$C logs <svc>`, or:
   ```sql
   SELECT Timestamp, ServiceName, Body, TraceId FROM otel_logs
   WHERE Body ILIKE '%<some marker>%' ORDER BY Timestamp DESC LIMIT 20;
   ```
2. Pull the spans:
   ```sql
   SELECT Timestamp, ServiceName, SpanName, Duration, StatusCode FROM otel_traces
   WHERE TraceId = '<id>' ORDER BY Timestamp;
   ```
3. Pull the logs for that trace:
   ```sql
   SELECT Timestamp, ServiceName, SeverityText, Body FROM otel_logs
   WHERE TraceId = '<id>' ORDER BY Timestamp;
   ```
   (`logs` / `http_logs` also carry trace context — see migration `011_add_trace_context_to_logs.sql`.)

## Recap / Acolyte degradation

```bash
uv run python scripts/investigate_recap_degradation.py
$C logs --since=1h --timestamps recap-worker recap-subworker recap-evaluator | grep -iE 'error|timeout|degraded|checkpoint'
```
```bash
# recap-db
$C exec recap-db sh -c 'psql -U "$RECAP_DB_USER" -d "$RECAP_DB_NAME" -P pager=off -c "\dt" -c "SELECT count(*), max(created_at) FROM <artefact-table>;"'
```
Runbooks: `docs/runbooks/acolyte-checkpoint-resume.md`, `docs/runbooks/3days-recap-artefact-recovery.md` (if present),
`docs/runbooks/acolyte-degraded-mode.md`, `docs/runbooks/acolyte-llm-timeout.md`.

## Existing scripts to reuse

| Script | Purpose |
|---|---|
| `scripts/analyze_docker_logs.py` | Parse `docker compose logs` output, extract error/warning patterns (esp. recap services) |
| `scripts/analyze_clickhouse_performance.py` | Platform health snapshot from ClickHouse logs/traces (`--hours N`, `--output-dir`) |
| `scripts/analyze-nginx-logs.sh` | nginx latency / status-code breakdown |
| `scripts/pgbouncer_stats.sh` | pgbouncer pool/stats dump |
| `scripts/investigate_recap_degradation.py` | Recap-worker degradation diagnostics |

Run these as **read-only** diagnostics; don't pass any write/cleanup flags.
