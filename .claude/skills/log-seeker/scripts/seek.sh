#!/usr/bin/env bash
# log-seeker bundler — collect a READ-ONLY diagnostic bundle from the Alt stack.
#
# Usage:
#   seek.sh [--since DUR] [--out DIR] [--tail N] [service ...]
#
#   --since DUR   time window for `docker compose logs` (default: 30m)
#   --out DIR     output directory (default: /tmp/log-seeker-<UTC timestamp>)
#   --tail N      max log lines per service (default: 2000 — enough to cover a
#                 normal-traffic window without flooding the bundle)
#   service ...   services to pull logs for; if none given, a curated core set is used
#
# It never mutates anything: only `docker compose ps/logs`, `docker inspect`, and
# SELECT/SHOW queries. Missing or stopped containers are noted and skipped, never fatal.
# Exits 0 even on partial failure and prints a summary plus the bundle path.

set -u  # (no `set -e`: partial collection is expected and fine)

COMPOSE=(docker compose -f compose/compose.yaml -p alt)
SINCE="30m"
TAIL=2000
OUT=""
DEFAULT_SERVICES=(nginx alt-backend alt-frontend-sv auth-hub pre-processor search-indexer mq-hub)
SERVICES=()
ERR_RE='ERROR|FATAL|panic|traceback|Traceback|OOMKilled|exit code [1-9]|exception|Exception'

while [ $# -gt 0 ]; do
  case "$1" in
    --since) SINCE="${2:?--since needs a value}"; shift 2 ;;
    --out)   OUT="${2:?--out needs a value}"; shift 2 ;;
    --tail)  TAIL="${2:?--tail needs a value}"; shift 2 ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    --) shift; while [ $# -gt 0 ]; do SERVICES+=("$1"); shift; done ;;
    -*) echo "seek.sh: unknown option: $1" >&2; exit 2 ;;
    *)  SERVICES+=("$1"); shift ;;
  esac
done
[ "${#SERVICES[@]}" -eq 0 ] && SERVICES=("${DEFAULT_SERVICES[@]}")
[ -n "$OUT" ] || OUT="/tmp/log-seeker-$(date -u +%Y%m%dT%H%M%SZ)"

if ! command -v docker >/dev/null 2>&1; then
  echo "seek.sh: docker not found on PATH" >&2; exit 2
fi
# Operate from the repo root (where compose/compose.yaml lives).
if [ ! -f compose/compose.yaml ]; then
  root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
  if [ -n "$root" ] && [ -f "$root/compose/compose.yaml" ]; then
    cd "$root" || { echo "seek.sh: cannot cd to repo root $root" >&2; exit 2; }
  else
    echo "seek.sh: run me from the Alt repo root (compose/compose.yaml not found)" >&2; exit 2
  fi
fi

mkdir -p "$OUT/logs"
SUMMARY="$OUT/SUMMARY.txt"
: > "$SUMMARY"
log() { echo "$@" | tee -a "$SUMMARY"; }

log "log-seeker bundle  $(date -u +%FT%TZ)"
log "since=$SINCE  tail=$TAIL  services=${SERVICES[*]}"
log "out=$OUT"
log ""

# --- running containers ------------------------------------------------------
running="$("${COMPOSE[@]}" ps --services --status running 2>/dev/null)"
is_running() { printf '%s\n' "$running" | grep -qx -- "$1"; }

"${COMPOSE[@]}" ps > "$OUT/ps.txt" 2>&1 || echo "(docker compose ps failed)" >> "$OUT/ps.txt"
unhealthy="$("${COMPOSE[@]}" ps 2>/dev/null | grep -iE 'restarting|unhealthy|exited' || true)"
if [ -n "$unhealthy" ]; then
  log "## Containers not healthy:"; log "$unhealthy"; log ""
else
  log "## All listed containers look healthy."; log ""
fi

# --- per-service logs --------------------------------------------------------
log "## Per-service error counts (window=$SINCE):"
for svc in "${SERVICES[@]}"; do
  out="$OUT/logs/$svc.log"
  if ! "${COMPOSE[@]}" logs --since="$SINCE" --timestamps --tail="$TAIL" "$svc" > "$out" 2>&1; then
    log "  $svc: (no such service / not started)"
    continue
  fi
  n="$(grep -Ec "$ERR_RE" "$out" 2>/dev/null)"; n="${n:-0}"
  lines="$(wc -l < "$out" | tr -d ' ')"
  log "  $svc: $n error-ish lines / $lines total"
  if [ "$n" -gt 0 ]; then
    {
      echo "### $svc ($n matches)"
      grep -nE "$ERR_RE" "$out" | tail -20
      echo
    } >> "$OUT/error-summary.txt"
  fi
  # crash/OOM detail if the compose container exists
  cid="$("${COMPOSE[@]}" ps -q "$svc" 2>/dev/null)"
  if [ -n "$cid" ]; then
    docker inspect --format '{{.Name}} RestartCount={{.RestartCount}} OOMKilled={{.State.OOMKilled}} ExitCode={{.State.ExitCode}} Status={{.State.Status}}' "$cid" \
      >> "$OUT/container-state.txt" 2>/dev/null || true
  fi
done
log ""

# --- ClickHouse: recent errors ----------------------------------------------
if is_running clickhouse; then
  ch() { "${COMPOSE[@]}" exec -T clickhouse sh -c \
    'clickhouse-client -u "$CLICKHOUSE_USER" --password "$(cat /run/secrets/clickhouse_password)" -d "$CLICKHOUSE_DB" --query "'"$1"'"' 2>&1; }
  {
    echo "# errors by service, last hour"
    ch "SELECT ServiceName, count() AS n FROM otel_error_logs WHERE Timestamp > now() - INTERVAL 1 HOUR GROUP BY ServiceName ORDER BY n DESC FORMAT PrettyCompact"
    echo
    echo "# latest 30 error rows"
    ch "SELECT Timestamp, ServiceName, SeverityText, substring(Body, 1, 200) AS body, TraceId FROM otel_error_logs WHERE Timestamp > now() - INTERVAL 1 HOUR ORDER BY Timestamp DESC LIMIT 30 FORMAT PrettyCompact"
  } > "$OUT/clickhouse-errors.txt"
  top_ch="$(ch "SELECT ServiceName || ':' || toString(count()) FROM otel_error_logs WHERE Timestamp > now() - INTERVAL 1 HOUR GROUP BY ServiceName ORDER BY count() DESC LIMIT 5 FORMAT TSV" | paste -sd' ' -)"
  log "## ClickHouse errors (last 1h, top services): ${top_ch:-none}"
else
  log "## ClickHouse: not running — skipped."
fi
log ""

# --- Postgres health: alt-db + knowledge-sovereign-db ------------------------
# psql via the container's own env (peer/trust on the local socket — no password).
pgq()  { "${COMPOSE[@]}" exec -T "$1" sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off -At -c "'"$2"'"' 2>/dev/null; }
pgtbl() { "${COMPOSE[@]}" exec -T "$1" sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off -c "'"$2"'"' 2>&1; }
digits_or() { case "${1:-}" in (*[!0-9]*|'') printf '%s' "$2";; (*) printf '%s' "$1";; esac; }

if is_running db; then
  {
    echo "# alt-db: connections by state"
    pgtbl db "SELECT state, count(*) FROM pg_stat_activity GROUP BY state ORDER BY 2 DESC;"
    echo
    echo "# alt-db: non-idle queries (longest first)"
    pgtbl db "SELECT pid, usename, application_name, now() - query_start AS dur, left(query,120) AS q FROM pg_stat_activity WHERE state <> 'idle' AND query_start IS NOT NULL ORDER BY dur DESC LIMIT 10;"
    echo
    echo "# alt-db: ungranted locks"
    pgtbl db "SELECT locktype, relation::regclass, mode, pid FROM pg_locks WHERE NOT granted;"
    echo
    echo "# alt-db: outbox backlog (unsent)"
    pgtbl db "SELECT count(*) AS unsent FROM outbox_events WHERE processed_at IS NULL;"
  } > "$OUT/pg-health.txt"
  log "## alt-db: see pg-health.txt"
else
  log "## alt-db: not running — skipped."
fi

if is_running knowledge-sovereign-db; then
  {
    echo "# knowledge-sovereign-db: projector checkpoints (lag)"
    pgtbl knowledge-sovereign-db "SELECT projector_name, last_event_seq, updated_at, EXTRACT(EPOCH FROM now() - updated_at)::int AS lag_seconds FROM knowledge_projection_checkpoints ORDER BY lag_seconds DESC;"
    echo
    echo "# knowledge-sovereign-db: home projection freshness"
    pgtbl knowledge-sovereign-db "SELECT count(*) AS items, max(updated_at) AS newest FROM knowledge_home_items;"
    echo
    echo "# knowledge-sovereign-db: event log size"
    pgtbl knowledge-sovereign-db "SELECT count(*) AS events FROM knowledge_events;"
  } > "$OUT/sovereign-health.txt"
  maxlag="$(pgq knowledge-sovereign-db "SELECT COALESCE(MAX(EXTRACT(EPOCH FROM now() - updated_at)::int),0) FROM knowledge_projection_checkpoints;" | tr -dc '0-9\n' | tail -1)"
  log "## knowledge-sovereign-db: max projector lag = $(digits_or "$maxlag" 'n/a')s"
else
  log "## knowledge-sovereign-db: not running — skipped."
fi
log ""

# --- pgbouncer pools ---------------------------------------------------------
if is_running pgbouncer; then
  "${COMPOSE[@]}" exec -T pgbouncer sh -c \
    'psql -h 127.0.0.1 -p 6432 -U "$DB_USER" pgbouncer -c "SHOW POOLS;" -c "SHOW STATS;" -c "SHOW CLIENTS;"' \
    > "$OUT/pgbouncer.txt" 2>&1 || true
  if grep -qiE 'error|FATAL|not allowed|denied' "$OUT/pgbouncer.txt"; then
    echo "(pgbouncer admin console not reachable as \$DB_USER — needs a stats/admin user; try: bash scripts/pgbouncer_stats.sh)" >> "$OUT/pgbouncer.txt"
    log "## pgbouncer: admin query failed (see pgbouncer.txt; try scripts/pgbouncer_stats.sh)"
  else
    log "## pgbouncer: see pgbouncer.txt"
  fi
else
  log "## pgbouncer: not running — skipped."
fi
log ""

# --- redis-streams quick depth ----------------------------------------------
if is_running redis-streams; then
  "${COMPOSE[@]}" exec -T redis-streams sh -c \
    'redis-cli INFO clients; echo "--- keys ---"; redis-cli --scan --pattern "*" | head -50' \
    > "$OUT/redis-streams.txt" 2>&1 || echo "(redis query failed)" >> "$OUT/redis-streams.txt"
  log "## redis-streams: see redis-streams.txt"
else
  log "## redis-streams: not running — skipped."
fi

log ""
log "Bundle written to: $OUT"
log "Files: $(cd "$OUT" && find . -type f | sed 's#^\./##' | paste -sd' ' -)"
echo ""
echo "==> $OUT"
exit 0
