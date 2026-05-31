#!/usr/bin/env bash
# Retention-policy guard for the ClickHouse `rask_logs` log/trace store.
#
# Asserts every log/trace table enforces the 24h retention policy
# (TTL = 1 day) while the derived SLI metrics keep their long trend window
# (90 days). Read-only: it only reads system.tables. Exits non-zero on any
# violation so it can run as a RED->GREEN guard and in CI.
#
# Override the compose handle / service name via env if needed:
#   COMPOSE="docker compose -f compose/compose.yaml -p alt" CH_SVC=clickhouse
set -euo pipefail

COMPOSE="${COMPOSE:-docker compose -f compose/compose.yaml -p alt}"
CH_SVC="${CH_SVC:-clickhouse}"

# table -> expected TTL in days
declare -A EXPECTED=(
  [otel_logs]=1
  [otel_traces]=1
  [otel_http_requests]=1
  [otel_error_logs]=1
  [http_logs]=1
  [logs]=1
  [sli_metrics]=90
)

names=""
for t in "${!EXPECTED[@]}"; do names+="'${t}',"; done
names="${names%,}"

query="SELECT name, engine_full FROM system.tables WHERE database = currentDatabase() AND name IN (${names}) FORMAT TabSeparated"

rows="$($COMPOSE exec -T "$CH_SVC" sh -c \
  "clickhouse-client -u \"\$CLICKHOUSE_USER\" --password \"\$(cat /run/secrets/clickhouse_password)\" -d \"\$CLICKHOUSE_DB\" -q \"${query}\"")"

fail=0
present=""
while IFS=$'\t' read -r name engine; do
  [ -z "${name:-}" ] && continue
  present+="${name}"$'\n'
  want="${EXPECTED[$name]:-}"
  [ -z "$want" ] && continue
  if printf '%s' "$engine" | grep -q "toIntervalDay($want)"; then
    echo "PASS  ${name}  TTL=${want}d"
  else
    got="$(printf '%s' "$engine" | grep -oE 'toIntervalDay\([0-9]+\)' || echo 'NONE')"
    echo "FAIL  ${name}  expected toIntervalDay(${want}), got: ${got}"
    fail=1
  fi
done <<EOF
${rows}
EOF

for t in "${!EXPECTED[@]}"; do
  if ! printf '%s' "$present" | grep -qx "$t"; then
    echo "FAIL  ${t}  table not found in rask_logs"
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  echo "RETENTION POLICY: VIOLATIONS FOUND"
  exit 1
fi
echo "RETENTION POLICY: OK (24h on logs/traces, 90d on sli_metrics)"
