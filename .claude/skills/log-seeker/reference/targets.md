# log-seeker — targets & connection recipes

What to inspect and exactly how to reach it. Everything here is **read-only**.

## Contents
- [Log pipeline topology](#log-pipeline-topology)
- [General rules](#general-rules)
- [Container logs](#container-logs)
- [PostgreSQL databases](#postgresql-databases)
- [Key tables](#key-tables)
- [ClickHouse (`rask_logs`)](#clickhouse-rask_logs)
- [pgbouncer (connection pools)](#pgbouncer-connection-pools)
- [Redis Streams (mq-hub queue)](#redis-streams-mq-hub-queue)
- [Meilisearch](#meilisearch)

## Log pipeline topology

```
container stdout/stderr
   → docker json-file logs (/var/lib/docker/containers/*)         ← `docker compose logs`
   → rask-log-forwarder (one per service, reads docker.sock)
   → rask-log-aggregator:9600 /v1/aggregate   (+ OTLP :4317 gRPC / :4318 HTTP)   ← INGEST ONLY
   → ClickHouse :8123 (HTTP) / :9009 (native)                     ← query the logs here
   → tables: logs, http_logs(+mv), otel_logs, otel_traces, otel_http_requests(+mv),
             otel_error_logs(+mv), sli_metrics(+sli_*_mv)
```

Forwarder services (all `ghcr.io/<owner>/alt-rask-log-forwarder`, in `compose/logging.yaml`):
`nginx-logs`, `alt-backend-logs`, `alt-harvester-logs`, `alt-data-hub-logs`, `auth-hub-logs`,
`tag-generator-logs`, `pre-processor-logs`, `search-indexer-logs`, `news-creator-logs`,
`news-creator-backend-logs`, `recap-worker-logs`, `recap-subworker-logs`, `recap-evaluator-logs`,
`dashboard-logs`, `rag-orchestrator-logs`, `mq-hub-logs`. The aggregator (`rask-log-aggregator`)
only exposes `/v1/health`, `/v1/aggregate`, and OTLP `/v1/logs` + `/v1/traces` — there is
**no log query API**; go to ClickHouse.

## General rules

- Derive identifiers from container env, never hardcode: `$POSTGRES_USER`, `$POSTGRES_DB`,
  `$CLICKHOUSE_USER`, `$CLICKHOUSE_DB`, `$RECAP_DB_USER`, etc.
- Read passwords only **inside** the container from `/run/secrets/<name>_password`. Never echo
  a secret; never put it on a host command line.
- Postgres containers trust the local unix socket (peer/trust) — `psql -U "$POSTGRES_USER"`
  works inside the container with no password and no `-h`.
- Mask production hostnames/domains in any quoted output.
- Compose handle used everywhere: `docker compose -f compose/compose.yaml -p alt …`
  (stacks are composed via `compose/compose.yaml` `include:` directives, not root-level `--profile` flags).

## Container logs

```bash
C="docker compose -f compose/compose.yaml -p alt"
$C ps                                                # status / health / restart counts
$C logs --since=30m --timestamps <svc>               # window
$C logs --tail=300 --timestamps <svc>                # last N lines
$C logs --since=10m --timestamps <svc> | grep -iE 'error|warn|panic|fatal|traceback|oom|exit code'
docker inspect --format '{{.RestartCount}} OOM={{.State.OOMKilled}} exit={{.State.ExitCode}}' <container_name>
```

## PostgreSQL databases

| Service / container | Port (host) | DB name var (default) | User var (default) | Password secret | Migrations dir | Owns |
|---|---|---|---|---|---|---|
| `db` / `alt-db` | 5432 | `$POSTGRES_DB` | `$POSTGRES_USER` (`alt_db_user`) | `postgres_password` | `migrations-atlas/migrations/` | RSS working set + summaries + reports + outbox |
| `knowledge-sovereign-db` | 5438 | `$POSTGRES_DB` (`knowledge_sovereign`) | `$POSTGRES_USER` (`sovereign`) | `sovereign_db_password` | `knowledge-sovereign/migrations/` | **the immutable knowledge model** (events, projections, OODA loop) |
| `pre-processor-db` | 5437 | `$POSTGRES_DB` (`pre_processor`) | `$POSTGRES_USER` (`pp_user`) | `pp_db_password` | `pre-processor-migration-atlas/migrations/` | pre-processor working set |
| `recap-db` | 5435 | `$POSTGRES_DB` (`recap`) | `$POSTGRES_USER` (`recap_user`) | `recap_db_password` | `recap-migration-atlas/migrations/` | 3-day recap artefacts |
| `rag-db` | 5436 | `$POSTGRES_DB` (`rag_db`) | `$POSTGRES_USER` (`rag_user`) | `rag_db_password` | `rag-migration-atlas/migrations/` | RAG / embeddings |
| `acolyte-db` | 5439 | `$POSTGRES_DB` (`acolyte`) | `$POSTGRES_USER` (`acolyte_user`) | `acolyte_db_password` | `acolyte-migration-atlas/migrations/` | Acolyte reports/checkpoints |
| `kratos-db` | 5434 | `$POSTGRES_DB` (`kratos`) | `$POSTGRES_USER` (`kratos_user`) | `kratos_db_password` | `kratos-db/init/` | identities (Ory Kratos); pooled via `pgbouncer-kratos` |
| `pact-db` | — | `$POSTGRES_DB` (`pact`) | `$POSTGRES_USER` (`pact`) | `pact_db_password` | — | Pact Broker state (always-on stack via `compose/pact.yaml`) |

Every one of these is a stock `postgres` image, so inside the container `$POSTGRES_USER`/`$POSTGRES_DB`
hold the right values — the connect recipe below is identical for all of them.

(Confirm the actual env values/ports in `compose/db.yaml`, `compose/recap.yaml`, `compose/rag.yaml`,
`compose/acolyte.yaml`, `compose/auth.yaml`, `compose/sovereign.yaml` — defaults above can drift.)

Connect (same for every Postgres DB — just swap the service name):

```bash
C="docker compose -f compose/compose.yaml -p alt"

# interactive
$C exec db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off'
$C exec knowledge-sovereign-db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off'
$C exec recap-db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off'

# one-shot query (note: keep SQL string literals in single quotes; they survive the outer sh -c)
$C exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off -c "SELECT now();"'

# list tables / describe one
$C exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "\dt"'
$C exec -T knowledge-sovereign-db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "\d knowledge_events"'
```

## Key tables

**alt-db** (RSS / summaries / reports): `feeds`, `feed_links`, `feed_link_availability`,
`articles`, `article_heads`, `article_summaries`, `article_tags`, `feed_tags`, `read_status`,
`user_reading_status`, `user_feed_subscriptions`, `favorite_feeds`, `summary_versions`,
`tag_set_versions`, `summarize_job_queue`, `outbox_events`, `image_proxy_cache`,
`scraping_domains`, `declined_domains`, and the `report*` family
(`reports`, `report_jobs`, `report_runs`, `report_versions`, `report_sections`,
`report_section_versions`, `report_change_items`).

**knowledge-sovereign-db** — the immutable knowledge / event-sourcing model (append-first;
read models are disposable projections, rebuildable from the event log):
- `knowledge_events` (partitioned, INSERT-only — the source of truth), `knowledge_user_events` (partitioned), `knowledge_event_dedupes`
- `knowledge_home_items` — Home read model; `today_digest_view`, `recall_candidate_view`, `recall_signals` — other read models
- `knowledge_lenses`, `knowledge_lens_versions`, `knowledge_current_lens`
- `knowledge_projection_checkpoints` (projector position + `updated_at` → lag), `knowledge_projection_audits`, `knowledge_projection_snapshots`, `knowledge_projection_versions`, `knowledge_reproject_runs`, `knowledge_retention_log`
- `knowledge_backfill_jobs`
- OODA loop: `knowledge_loop_entries`, `knowledge_loop_surfaces`, `knowledge_loop_session_state`, `knowledge_loop_entry_session_state`, `knowledge_loop_transition_dedupes`
- `knowledge_user_event_aggregates`

(`recap-db`, `rag-db`, `acolyte-db`, `pre-processor-db`, `kratos-db` own their own domains —
run `\dt` to see them.)

Useful read-only checks (run against `knowledge-sovereign-db` for the knowledge model,
against `db` for the rest):

```sql
-- projector lag (knowledge-sovereign-db)
SELECT projector_name, last_event_seq, updated_at,
       EXTRACT(EPOCH FROM now() - updated_at)::int AS lag_seconds
FROM knowledge_projection_checkpoints ORDER BY lag_seconds DESC;

-- Home projection freshness (knowledge-sovereign-db)
SELECT count(*) AS items, max(updated_at) AS newest FROM knowledge_home_items;

-- outbox backlog (alt-db)
SELECT count(*) AS unsent FROM outbox_events WHERE processed_at IS NULL;

-- connections / long-running queries / locks (any DB)
SELECT state, count(*) FROM pg_stat_activity GROUP BY state;
SELECT pid, usename, now() - query_start AS dur, left(query, 120) AS q
FROM pg_stat_activity WHERE state <> 'idle' AND query_start IS NOT NULL ORDER BY dur DESC LIMIT 10;
SELECT * FROM pg_locks WHERE NOT granted;
```

## ClickHouse (`rask_logs`)

Container/service `clickhouse` — HTTP `8123`, native `9009`. DB `$CLICKHOUSE_DB` (`rask_logs`),
user `$CLICKHOUSE_USER`, password `/run/secrets/clickhouse_password`. **TTL ≈ 1 day** on
`logs`, `http_logs`, `otel_logs`, `otel_traces` — older data is dropped.

Tables: `logs`, `http_logs` (+`http_logs_mv`), `otel_logs`, `otel_traces`,
`otel_http_requests` (+`otel_http_requests_mv`), `otel_error_logs` (+`otel_error_logs_mv`),
`sli_metrics` (+`sli_error_rate_mv`, `sli_log_throughput_mv`). Note column casing differs:
`logs`/`http_logs` use `timestamp`; OTel tables use `Timestamp` / `ServiceName` / `TraceId`
(check `clickhouse/migrations/*.sql` if a column name fails).

Connect:

```bash
# interactive
docker compose -f compose/compose.yaml -p alt exec clickhouse \
  sh -c 'clickhouse-client -u "$CLICKHOUSE_USER" --password "$(cat /run/secrets/clickhouse_password)" -d "$CLICKHOUSE_DB"'

# one-shot query
docker compose -f compose/compose.yaml -p alt exec -T clickhouse \
  sh -c 'clickhouse-client -u "$CLICKHOUSE_USER" --password "$(cat /run/secrets/clickhouse_password)" -d "$CLICKHOUSE_DB" -q "SHOW TABLES"'

# show a table definition (to confirm column names)
... -q "SHOW CREATE TABLE otel_logs"
```

## pgbouncer (connection pools)

`pgbouncer` (txn pooling, listens TCP `6432`, `pool_mode=transaction`, `default_pool_size=30`);
`pgbouncer-kratos` is the kratos one. **Heads-up:** in this stack `admin_users = postgres` but
there is no `postgres` entry in `userlist.txt`, so the `pgbouncer` admin console
(`SHOW POOLS/STATS/CLIENTS`) is effectively unreachable. The practical signals are instead:

```bash
C="docker compose -f compose/compose.yaml -p alt"
$C logs --since=30m --timestamps pgbouncer        # pool exhaustion / auth errors show up here
# pool pressure as seen from the backing Postgres:
$C exec -T db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT state, count(*) FROM pg_stat_activity GROUP BY 1;" -c "SHOW max_connections;"'
# if the admin console *is* provisioned in your env, this is the shape (will say FATAL: not allowed otherwise):
$C exec -T pgbouncer sh -c 'psql -h 127.0.0.1 -p 6432 -U "$DB_USER" pgbouncer -c "SHOW POOLS;" -c "SHOW STATS;"'
# existing helper (also depends on a usable admin user):
bash scripts/pgbouncer_stats.sh
```

## Redis Streams (mq-hub queue)

Service/container `redis-streams` (host port `6380` → 6379). Redis runs ACLs
(`docker/redis/entrypoint.sh`) with `user default off`, so an unauthenticated `redis-cli` —
`ping` included — gets `NOAUTH`. Authenticate as a role user with the password read from the
container's own secret file; `REDISCLI_AUTH` keeps it off argv and out of the output.

| Role | Keys | Read-only commands that work |
| --- | --- | --- |
| `streams` (`/run/secrets/redis_streams_password`) | `alt:events:*`, `alt:replies:tags:*` | `PING`, `SCAN` (`--scan`), `XINFO STREAM`, `XINFO GROUPS`, `XPENDING`, `TTL` |
| `limiter` (`/run/secrets/redis_limiter_password`, DB 3) | `host_rate_limiter:v1:*` | `PING`, `PTTL` |

`INFO`, `XLEN`, `XRANGE`, `TYPE`, `DBSIZE` and `XINFO CONSUMERS` are `NOPERM` for every role:
read the stream length from `XINFO STREAM` (`length`) instead of `XLEN`, and per-group backlog
from `XINFO GROUPS` (`pending`, `lag`). The roles also hold write commands (`XADD`, `XACK`,
`XTRIM`, `DEL`, `SET`, …) — never run them.

```bash
rs() { docker compose -f compose/compose.yaml -p alt exec -T redis-streams sh -c \
  'REDISCLI_AUTH="$(cat /run/secrets/redis_streams_password)" redis-cli --user streams "$@"' rs "$@"; }
rs ping
rs --scan --pattern 'alt:events:*' | head
rs XINFO STREAM <stream-key>        # length, groups, last-generated-id (then entry payloads)
rs XINFO GROUPS <stream-key>        # per group: pending, last-delivered-id, lag
rs XPENDING <stream-key> <group>    # pending summary; append `- + 10` for the oldest entries
```

(`mq-hub` reads `REDIS_URL=redis://streams@redis-streams:6379` + `REDIS_PASSWORD_FILE`; check
`compose/mq.yaml` for stream/group names.)

`redis-cache` (news-creator LLM cache, `alt-network` only) has a single `cache` role
(`/run/secrets/redis_cache_password`, keys `recap_card:*` / `recap:summary:*`) limited to
`PING` / `GET` (plus `SET` / `DEL`, never run). `SCAN`, `INFO` and `TTL` are `NOPERM`, so only
a known key can be read:

```bash
docker compose -f compose/compose.yaml -p alt exec -T redis-cache sh -c \
  'REDISCLI_AUTH="$(cat /run/secrets/redis_cache_password)" redis-cli --user cache ping'
```

## Meilisearch

Service `meilisearch`, host port `7700`, master key `/run/secrets/meili_master_key`.

```bash
curl -s http://localhost:7700/health
docker compose -f compose/compose.yaml -p alt exec -T meilisearch \
  sh -c 'curl -s -H "Authorization: Bearer $(cat /run/secrets/meili_master_key)" http://localhost:7700/stats'
# also: /indexes , /tasks?statuses=failed
```
