#!/usr/bin/env bash
# Shape guard for the edge-HTTP-access-log materialized view.
#
# http_logs_mv silently stopped populating http_logs for ~4 weeks after the
# nginx -> plecto-proxy edge cutover: the MV filtered on
# service_name = 'nginx' AND fields['http_method'] != '', but plecto-proxy
# logs service_name = 'plecto-proxy' with bare field keys
# (method/path/status/duration_ms/client), not nginx's http_-prefixed keys.
# Two independent, silent mismatches -- the MV just matched zero rows.
#
# A later fix swapped in plecto-proxy-only matching, which trades that bug
# for the same failure mode against nginx (or any future rollback). The
# current migration matches BOTH producer shapes, each gated on its own
# service_name, so this guard checks for both branches, not the presence
# of one over the other.
#
# This is a static text check against the migration files, not a live
# ClickHouse query: the fix only edits files (no DDL is run against the
# live cluster), so the guard must be able to go RED -> GREEN from file
# edits alone.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MV_FILE="${REPO_ROOT}/clickhouse/migrations/003_create_http_logs_mv.sql"

fail=0

check() {
  local desc="$1" cond="$2"
  if eval "$cond"; then
    echo "PASS  ${desc}"
  else
    echo "FAIL  ${desc}"
    fail=1
  fi
}

mv_sql="$(cat "$MV_FILE")"

# Strip `--` line comments before any content check below. The file's own
# prose legitimately contains the literal strings this guard looks for
# (e.g. "service_name='nginx'" appears in a comment explaining the outage
# history), so checking the raw file text makes the guard a false-positive
# trap on its own documentation. All content checks below run against
# mv_code, never mv_sql directly.
mv_code="$(sed -E 's/--.*$//' <<<"$mv_sql")"

# Must gate on BOTH known producers' service_name, not either one alone --
# that is exactly what made this view blind first to plecto-proxy (nginx
# hardcoded) and then to nginx (plecto-proxy hardcoded). Whitespace around
# `=` must not matter: the original outage's exact WHERE clause used no
# spaces (service_name='nginx'), and a guard that only matches spaced `=`
# reintroduces the outage undetected.
check "http_logs_mv matches nginx's service_name (whitespace-insensitive)" \
  "grep -qE \"service_name[[:space:]]*=[[:space:]]*'nginx'\" <<<\"\$mv_code\""
check "http_logs_mv matches plecto-proxy's service_name (whitespace-insensitive)" \
  "grep -qE \"service_name[[:space:]]*=[[:space:]]*'plecto-proxy'\" <<<\"\$mv_code\""

# Must key on the actual field shape each producer emits: nginx's
# http_-prefixed keys and plecto-proxy's bare keys, both present at once
# (see check above -- dropping either producer's fields here is the same
# bug as dropping its service_name branch).
for key in method path status client duration_ms; do
  check "http_logs_mv references plecto-proxy's fields['${key}']" \
    "grep -qF \"fields['${key}']\" <<<\"\$mv_code\""
done
for key in http_method http_path http_status http_ip http_ua; do
  check "http_logs_mv references nginx's fields['${key}']" \
    "grep -qF \"fields['${key}']\" <<<\"\$mv_code\""
done

# Every downstream SLO query (observability/grafana/dashboards/
# slo-overview.json) aggregates FROM http_logs with no service_name
# filter of its own -- it trusts http_logs to contain only genuine edge
# access-log rows. A WHERE clause that matches on field shape alone, with
# no service_name qualifier at all, would let any unrelated log line that
# happens to carry a `method`/`path`/`status` field corrupt the
# availability SLO. Each producer branch must AND its shape check against
# its own service_name, so require at least two distinct service_name
# equality checks in the code (one per producer) rather than a shape-only
# WHERE with none.
service_name_filters="$(grep -oE "service_name[[:space:]]*=[[:space:]]*'[^']*'" <<<"$mv_code" | sort -u | wc -l)"
check "http_logs_mv WHERE clause has a service_name qualifier per producer (found ${service_name_filters})" \
  '[ "$service_name_filters" -ge 2 ]'

# http_logs had no column to hold latency; plecto-proxy's duration_ms is
# the only source of edge-request latency now that otel_http_requests_mv
# has no OTel-instrumented producer to read from.
check "http_logs gains a duration_ms column for latency SLIs" \
  'grep -qi "ADD COLUMN IF NOT EXISTS duration_ms" <<<"$mv_code"'

# CREATE MATERIALIZED VIEW IF NOT EXISTS is a permanent no-op once a view
# of this name already exists on a running cluster, so a corrected
# definition here must DROP the stale view first to actually take effect
# on redeploy.
check "http_logs_mv migration drops the view before recreating it" \
  'grep -qi "DROP VIEW IF EXISTS http_logs_mv" <<<"$mv_code"'

# No shell script that talks to ClickHouse may carry its own separate copy
# of this DDL: that copy ran AFTER the migrations loop on every restart and
# silently won, which is why re-running migrations alone could never fix
# this. Match CREATE MATERIALIZED VIEW ... http_logs_mv with the `IF NOT
# EXISTS` clause optional and whitespace/case-insensitive -- a duplicate
# copy re-pasted without "IF NOT EXISTS" (or with different spacing/casing)
# is the same root cause and must still be caught.
#
# The glob covers every clickhouse/*.sh, not just entrypoint-wrapper.sh:
# the migrations loop is now shared with the clickhouse-migrator one-shot
# (which runs the entrypoint in `apply` mode), and a re-pasted copy could
# land in any script added alongside it.
# shellcheck disable=SC2034  # used inside eval'd check() conditions below
shell_code="$(sed -E 's/#.*$//' "${REPO_ROOT}"/clickhouse/*.sh)"
check "clickhouse/*.sh does not embed its own http_logs_mv CREATE" \
  '! grep -qiE "CREATE[[:space:]]+MATERIALIZED[[:space:]]+VIEW([[:space:]]+IF[[:space:]]+NOT[[:space:]]+EXISTS)?[[:space:]]+http_logs_mv" <<<"$shell_code"'
# Belt-and-suspenders: also reject a duplicate SELECT body even if it were
# ever pasted in outside a CREATE MATERIALIZED VIEW statement (e.g. inside
# an INSERT ... SELECT). generateUUIDv4() AS log_id is unique to this MV's
# SELECT list in this file.
check "clickhouse/*.sh does not embed the http_logs_mv SELECT body" \
  '! grep -qF "generateUUIDv4() AS log_id" <<<"$shell_code"'

if [ "$fail" -ne 0 ]; then
  echo "HTTP LOGS MV SHAPE: VIOLATIONS FOUND"
  exit 1
fi
echo "HTTP LOGS MV SHAPE: OK"
