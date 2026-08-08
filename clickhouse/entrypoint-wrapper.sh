#!/bin/bash
set -e

# ClickHouse entrypoint and schema applier.
#
# Two modes, one implementation, deliberately:
#
#   server   (default, no argv)  start the ClickHouse server, apply the repo's
#                                schema against it, then hand the process back
#                                to the server.
#   apply    (argv is `apply`)   apply the repo's schema against an
#                                already-running server named by
#                                CLICKHOUSE_MIGRATE_HOST, then exit. This is
#                                the mode the one-shot `clickhouse-migrator`
#                                compose service runs, and the mode
#                                alt-deploy's playbooks/apply-migrations.yml
#                                invokes before it rolls any service.
#
# Why the apply mode has to exist: the release pipeline never recreates the
# clickhouse container. ClickHouse runs an upstream image, so it is in no
# service's roll set, and docs/runbooks/deploy.md keeps the databases out of
# recreate on purpose. A change under clickhouse/migrations/ therefore reached
# production only when a human happened to restart ClickHouse — which is how
# http_logs_mv could sit broken for weeks with the corrected SQL already
# merged.
#
# Why both modes share one code path: a second copy of the DDL-applying logic
# is the root cause this file has already been burned by. The entrypoint used
# to carry its own CREATE MATERIALIZED VIEW http_logs_mv, it ran after the
# migrations loop, and it always won — so fixing
# migrations/003_create_http_logs_mv.sql alone changed nothing on restart.
# A migrator that applied a *different* set of statements than the container
# start-up path would recreate that failure mode with the loud gate in
# alt-deploy turned into a silent success.

CH_HOST="${CLICKHOUSE_MIGRATE_HOST:-localhost}"
CH_PASSWORD_FILE="${CLICKHOUSE_PASSWORD_FILE:-/run/secrets/clickhouse_password}"
MIGRATIONS_DIR="${CLICKHOUSE_MIGRATIONS_DIR:-/migrations}"

# Required config, checked before anything connects: an unset user or database
# would otherwise reach clickhouse-client as an empty string and fail as an
# opaque auth or "database does not exist" error.
: "${CLICKHOUSE_USER:?CLICKHOUSE_USER must be set}"
: "${CLICKHOUSE_DB:?CLICKHOUSE_DB must be set}"
if [ ! -r "$CH_PASSWORD_FILE" ]; then
    echo "${CH_PASSWORD_FILE} is missing or unreadable — check the clickhouse_password secret mount." >&2
    exit 1
fi

ch() {
    clickhouse-client --host "$CH_HOST" --user "${CLICKHOUSE_USER}" \
        --password "$(cat "$CH_PASSWORD_FILE")" "$@"
}

# $1 = seconds to wait, or 0 to wait indefinitely. Server mode waits forever
# (a slow part-recovery on a multi-GB store must not turn into a restart loop);
# the migrator bounds the wait so an unreachable or unauthenticated server
# fails the deploy step loudly instead of hanging the job.
wait_for_clickhouse() {
    local timeout="$1" waited=0
    echo "Waiting for ClickHouse at ${CH_HOST} to accept queries..."
    until ch --query "SELECT 1" &>/dev/null; do
        if [ "$timeout" -ne 0 ] && [ "$waited" -ge "$timeout" ]; then
            echo "ClickHouse at ${CH_HOST} did not accept a query within ${timeout}s" >&2
            return 1
        fi
        sleep 1
        waited=$((waited + 1))
    done
    echo "ClickHouse is ready"
}

apply_schema() {
    echo "Creating database ${CLICKHOUSE_DB} if absent..."
    ch --query "CREATE DATABASE IF NOT EXISTS ${CLICKHOUSE_DB}"

    # Kill any stuck mutations before the migrations so DDL like `ALTER TABLE …
    # DROP COLUMN` in migration 010 is not blocked by a prior unfinished
    # mutation. Stuck mutations beyond a few minutes do not make progress on
    # their own; manually killing them is the documented escape hatch (see
    # ClickHouse error code 36 message). Skipped silently when there are none.
    echo "Killing any stuck mutations in ${CLICKHOUSE_DB}..."
    ch --query "KILL MUTATION WHERE database='${CLICKHOUSE_DB}' AND is_done=0 SYNC FORMAT Null" || true

    # There is no schema-revision table: every file is replayed, in filename
    # order, on every invocation. Each one must therefore be idempotent —
    # CREATE … IF NOT EXISTS, ADD/DROP COLUMN IF [NOT] EXISTS, or an explicit
    # DROP before CREATE for a view whose SELECT must be allowed to change
    # (CREATE MATERIALIZED VIEW IF NOT EXISTS is a permanent no-op once the
    # view exists, which is what hid the http_logs_mv breakage).
    local files=()
    while IFS= read -r f; do
        files+=("$f")
    done < <(find "$MIGRATIONS_DIR" -maxdepth 1 -type f -name '*.sql' | sort)

    # Fail closed on an empty directory. A missing or misdirected bind mount
    # would otherwise let this report success having applied nothing, which is
    # strictly worse than refusing: the deploy gate in alt-deploy exists to
    # stop exactly that "reported applied, actually skipped" shape.
    if [ "${#files[@]}" -eq 0 ]; then
        echo "No .sql files under ${MIGRATIONS_DIR} — refusing to report a successful apply." >&2
        echo "Check the clickhouse/migrations bind mount." >&2
        return 1
    fi

    echo "Running migrations..."
    for f in "${files[@]}"; do
        echo "Applying: $f"
        ch --database "${CLICKHOUSE_DB}" --multiquery < "$f"
    done
    echo "Migrations completed (${#files[@]} file(s))"
}

# One-time fix for the legacy `logs` table, deliberately NOT part of
# apply_schema: it drops and recreates a table that holds live rows, so it
# belongs to a container that owns the server, not to a migrator the deploy
# pipeline runs while log ingestion is in flight. It also cannot live in
# migrations/ at all, because every file there is replayed on every start and
# an unconditional DROP TABLE logs would destroy the retention window each
# time.
#
# The table was created with a non-temporal PARTITION BY (service_group,
# service_name): with ttl_only_drop_parts=1 every part keeps mixing fresh and
# old rows, so the 1-day TTL can never drop a part and "delete old data"
# silently never happens. Rebuild it once with a date-aligned partition so the
# retention policy actually applies. http_logs_mv reads FROM logs and must be
# recreated because the rebuilt table gets a new UUID. The partition_key guard
# makes this run exactly once and stay reboot-safe.
rebuild_legacy_logs_table() {
    local logs_pk
    logs_pk="$(ch --database "${CLICKHOUSE_DB}" --query \
        "SELECT partition_key FROM system.tables WHERE database='${CLICKHOUSE_DB}' AND name='logs'" 2>/dev/null || true)"
    # The broken partition key references service_group; the fixed one is
    # toDate(timestamp), so a substring match fires exactly once.
    case "$logs_pk" in
        *service_group*) ;;
        *) return 0 ;;
    esac

    echo "Rebuilding legacy 'logs' table with date-aligned partition (one-time)..."
    ch --database "${CLICKHOUSE_DB}" --multiquery <<'SQL'
DROP VIEW IF EXISTS http_logs_mv;
DROP TABLE IF EXISTS logs;
CREATE TABLE IF NOT EXISTS logs (
    service_type LowCardinality(String),
    log_type LowCardinality(String),
    message String,
    level Enum8('Debug' = 0, 'Info' = 1, 'Warn' = 2, 'Error' = 3, 'Fatal' = 4),
    timestamp DateTime64(3, 'UTC'),
    stream LowCardinality(String),
    container_id String,
    service_name LowCardinality(String),
    service_group LowCardinality(String),
    TraceId FixedString(32) DEFAULT '',
    SpanId FixedString(16) DEFAULT '',
    fields Map(String, String)
) ENGINE = MergeTree()
PARTITION BY toDate(timestamp)
ORDER BY (timestamp)
TTL timestamp + INTERVAL 1 DAY DELETE
SETTINGS ttl_only_drop_parts = 1, index_granularity = 8192;
SQL
    # http_logs_mv was just dropped above because it pointed at the old `logs`
    # table's UUID. Re-source the migration file instead of embedding a second
    # copy of its DDL here.
    echo "Re-applying http_logs_mv from migrations/003_create_http_logs_mv.sql..."
    ch --database "${CLICKHOUSE_DB}" --multiquery < "${MIGRATIONS_DIR}/003_create_http_logs_mv.sql"
    echo "'logs' table rebuilt with date-aligned partition."
}

mode=server
if [ "${1:-}" = "apply" ]; then
    mode=apply
    shift
fi

# A container configured as a migrator (CLICKHOUSE_MIGRATE_HOST points at
# another host) that was not asked to apply would otherwise start a second
# ClickHouse server and hang the deploy step. Refuse instead of guessing.
if [ "$mode" = server ] && [ -n "${CLICKHOUSE_MIGRATE_HOST:-}" ] && [ "$CLICKHOUSE_MIGRATE_HOST" != localhost ]; then
    echo "CLICKHOUSE_MIGRATE_HOST=${CLICKHOUSE_MIGRATE_HOST} marks this container as a migrator," >&2
    echo "but no 'apply' command was given. Refusing to start a server here." >&2
    exit 2
fi

if [ "$mode" = apply ]; then
    wait_for_clickhouse "${CLICKHOUSE_READY_TIMEOUT:-120}"
    apply_schema
    exit 0
fi

# Start ClickHouse in background
/entrypoint.sh "$@" &
CLICKHOUSE_PID=$!

wait_for_clickhouse 0
apply_schema
rebuild_legacy_logs_table

# Wait for ClickHouse process
wait $CLICKHOUSE_PID
