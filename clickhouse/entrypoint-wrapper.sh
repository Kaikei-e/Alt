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
echo "Migrations completed"

# Wait for ClickHouse process
wait $CLICKHOUSE_PID
