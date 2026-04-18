#!/usr/bin/env bash
# e2e/hurl/knowledge-sovereign/run.sh
#
# Brings up the knowledge-sovereign slice of the alt-staging stack
# (Postgres + Atlas migrator + sovereign service), runs the Hurl suite
# inside the alt-staging network (Hurl's Docker image joins the network
# so the `knowledge-sovereign` service DNS resolves), and tears down.
#
# The staging network is `internal: true`, which silently ignores host
# port publishes. Running Hurl inside the network is the only portable
# way to reach the SUT, and matches the mq-hub / search-indexer pattern.
#
# Environment overrides:
#   BASE_URL        — sovereign RPC URL as seen from the Hurl container
#                     (default: http://knowledge-sovereign:9500)
#   METRICS_URL     — sovereign admin+health URL
#                     (default: http://knowledge-sovereign:9501)
#   HURL_IMAGE      — Hurl container image (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   RUN_ID          — unique run identifier for dedupe_key isolation
#                     (default: $(date +%s))
#   KEEP_STACK=1    — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${BASE_URL:=http://knowledge-sovereign:9500}"
: "${METRICS_URL:=http://knowledge-sovereign:9501}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${RUN_ID:=$(date +%s)}"

REPORT_DIR="$ROOT/e2e/reports/knowledge-sovereign-$RUN_ID"
mkdir -p "$REPORT_DIR"

cleanup() {
  if [[ "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> tearing down alt-staging stack" >&2
    docker compose -f compose/compose.staging.yaml -p alt-staging \
      down -v --remove-orphans >/dev/null 2>&1 || true
  else
    echo "==> KEEP_STACK=1 — leaving alt-staging stack up" >&2
  fi
}
trap cleanup EXIT

# Stable, run-scoped UUIDs. Sovereign rejects non-UUID tenant/user/lens IDs,
# the knowledge_event_dedupes table enforces global uniqueness on dedupe_key,
# and FK constraints couple lens_version_id to lens_id. Generate once here
# and inject into every Hurl scenario via --variable.
uuid() { cat /proc/sys/kernel/random/uuid; }

TENANT_ID="$(uuid)"
USER_ID="$(uuid)"
EVENT_ID="$(uuid)"
LENS_ID="$(uuid)"
LENS_VERSION_ID="$(uuid)"
SIGNAL_ID="$(uuid)"

# Today's date (YYYY-MM-DD) for GetTodayDigest and occurred_at partitioning.
TODAY="$(date -u +%Y-%m-%d)"
OCCURRED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "==> bringing up knowledge-sovereign staging slice" >&2
docker compose -f compose/compose.staging.yaml -p alt-staging \
  --profile knowledge-sovereign \
  up -d --wait knowledge-sovereign-db knowledge-sovereign

# Run Hurl inside the alt-staging network. Mount the repo at the same
# absolute path so any `file,e2e/fixtures/...;` body resolves via
# --file-root "$ROOT".
hurl_run() {
  docker run --rm \
    --network alt-staging \
    -v "$ROOT:$ROOT" \
    -w "$ROOT" \
    "$HURL_IMAGE" \
    "$@"
}

common_vars=(
  --variable "base_url=$BASE_URL"
  --variable "metrics_url=$METRICS_URL"
  --variable "run_id=$RUN_ID"
  --variable "tenant_id=$TENANT_ID"
  --variable "user_id=$USER_ID"
  --variable "event_id=$EVENT_ID"
  --variable "lens_id=$LENS_ID"
  --variable "lens_version_id=$LENS_VERSION_ID"
  --variable "signal_id=$SIGNAL_ID"
  --variable "today=$TODAY"
  --variable "occurred_at=$OCCURRED_AT"
)

echo "==> running Hurl setup (serial)" >&2
hurl_run --test \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  e2e/hurl/knowledge-sovereign/00-setup.hurl

echo "==> running Hurl suite (serial; event_seq + lens FK require ordering)" >&2
# --jobs 1 forces a single worker. Hurl 7.1 --test defaults to parallel,
# which breaks this suite: CreateLens → CreateLensVersion → Select →
# ListLenses, and AppendKnowledgeEvent → CreateSnapshot, require strict
# ordering.
hurl_run --test \
  --jobs 1 \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  e2e/hurl/knowledge-sovereign/0[1-9]-*.hurl \
  e2e/hurl/knowledge-sovereign/1[0-9]-*.hurl

echo "==> suite passed. reports: $REPORT_DIR" >&2
