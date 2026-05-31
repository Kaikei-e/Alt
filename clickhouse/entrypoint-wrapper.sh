#!/bin/bash
set -e

# Start ClickHouse in background
/entrypoint.sh "$@" &
CLICKHOUSE_PID=$!

# Wait for ClickHouse to be ready
echo "Waiting for ClickHouse to start..."
until clickhouse-client --user "${CLICKHOUSE_USER}" --password "$(cat /run/secrets/clickhouse_password)" --query "SELECT 1" &>/dev/null; do
    sleep 1
done
echo "ClickHouse is ready"

# Create database if not exists
echo "Creating database ${CLICKHOUSE_DB}..."
clickhouse-client --user "${CLICKHOUSE_USER}" --password "$(cat /run/secrets/clickhouse_password)" --query "CREATE DATABASE IF NOT EXISTS ${CLICKHOUSE_DB}"

# Kill any stuck mutations on rask_logs before migrations so DDL like
# `ALTER TABLE … DROP COLUMN` in migration 010 is not blocked by a prior
# unfinished mutation. Stuck mutations beyond a few minutes do not make
# progress on their own; manually killing them is the documented
# escape hatch (see ClickHouse error code 36 message). Skipped silently
# when there are no stuck mutations.
echo "Killing any stuck mutations in ${CLICKHOUSE_DB}..."
clickhouse-client --user "${CLICKHOUSE_USER}" --password "$(cat /run/secrets/clickhouse_password)" \
    --query "KILL MUTATION WHERE database='${CLICKHOUSE_DB}' AND is_done=0 SYNC FORMAT Null" || true

# Run migrations
echo "Running migrations..."
for f in /migrations/*.sql; do
    if [ -f "$f" ]; then
        echo "Applying: $f"
        clickhouse-client --user "${CLICKHOUSE_USER}" --password "$(cat /run/secrets/clickhouse_password)" --database "${CLICKHOUSE_DB}" --multiquery < "$f"
    fi
done

# One-time fix for the legacy `logs` table. It was created with a non-temporal
# PARTITION BY (service_group, service_name): with ttl_only_drop_parts=1 every
# part keeps mixing fresh and old rows, so the 1-day TTL can never drop a part
# and "delete old data" silently never happens. Rebuild it once with a
# date-aligned partition so the retention policy actually applies. The table is
# empty/dormant (live logs flow to otel_logs); http_logs_mv reads FROM logs and
# must be recreated because the rebuilt table gets a new UUID. The partition_key
# guard makes this run exactly once and stay reboot-safe.
LOGS_PK="$(clickhouse-client --user "${CLICKHOUSE_USER}" --password "$(cat /run/secrets/clickhouse_password)" \
    --database "${CLICKHOUSE_DB}" --query \
    "SELECT partition_key FROM system.tables WHERE database='${CLICKHOUSE_DB}' AND name='logs'" 2>/dev/null || true)"
# The broken partition key references service_group; the fixed one is
# toDate(timestamp), so a substring match fires exactly once.
case "$LOGS_PK" in *service_group*)
    echo "Rebuilding legacy 'logs' table with date-aligned partition (one-time)..."
    clickhouse-client --user "${CLICKHOUSE_USER}" --password "$(cat /run/secrets/clickhouse_password)" \
        --database "${CLICKHOUSE_DB}" --multiquery <<'SQL'
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
CREATE MATERIALIZED VIEW IF NOT EXISTS http_logs_mv
TO http_logs
AS
SELECT
    generateUUIDv4() AS log_id,
    timestamp,
    fields['http_method'] AS method,
    fields['http_path'] AS path,
    toUInt16OrZero(fields['http_status']) AS status_code,
    toUInt64OrZero(fields['http_size']) AS response_size,
    fields['http_ip'] AS ip_address,
    fields['http_ua'] AS user_agent,
    service_name,
    container_id
FROM logs
WHERE service_name = 'nginx'
  AND mapContains(fields, 'http_method')
  AND fields['http_method'] != '';
SQL
    echo "'logs' table rebuilt with date-aligned partition."
    ;;
esac

echo "Migrations completed"

# Wait for ClickHouse process
wait $CLICKHOUSE_PID
