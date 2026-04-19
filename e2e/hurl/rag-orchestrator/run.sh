#!/usr/bin/env bash
# e2e/hurl/rag-orchestrator/run.sh
#
# Brings up the rag-orchestrator slice of the alt-staging stack
# (rag-db Postgres + pgvector, Atlas migrator, rag-orchestrator), seeds
# augur conversation history via psql inside the rag-db container, and
# runs the Hurl suite from inside the alt-staging network so the service
# DNS names (`rag-orchestrator`, `rag-db`) resolve.
#
# Hurl cannot issue SQL directly, so the seed step runs `psql` via
# `docker compose exec` between `up --wait` and the Hurl invocation.
# Variables committed to e2e/fixtures/rag-orchestrator/ feed both psql
# (via -v) and Hurl (via --variable), so the two halves agree on the
# seeded UUIDs without any runtime capture.
#
# Environment overrides:
#   BASE_URL              — rag-orchestrator REST URL as seen from the
#                           Hurl container (default: http://rag-orchestrator:9010)
#   CONNECT_URL           — rag-orchestrator Connect-RPC URL, h2c
#                           (default: http://rag-orchestrator:9011)
#   HURL_IMAGE            — Hurl container image
#                           (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   IMAGE_TAG             — Docker tag for the rag-orchestrator image (default: ci)
#   GHCR_OWNER            — GHCR namespace (default: kaikei-e)
#   RUN_ID                — unique run identifier (default: $(date +%s))
#   STAGING_PROJECT_NAME  — compose project + network name (default: alt-staging).
#                           CI sets alt-staging-rag-orchestrator so parallel
#                           matrix jobs on the shared Docker daemon don't collide.
#   KEEP_STACK=1          — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${BASE_URL:=http://rag-orchestrator:9010}"
: "${CONNECT_URL:=http://rag-orchestrator:9011}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${IMAGE_TAG:=ci}"
: "${GHCR_OWNER:=kaikei-e}"
: "${RUN_ID:=$(date +%s)}"
: "${STAGING_PROJECT_NAME:=alt-staging}"

export IMAGE_TAG GHCR_OWNER STAGING_PROJECT_NAME

# Render a per-project compose slice (sets $SLICE + $SLICE_DIR) so
# parallel matrix jobs can coexist on the same Docker daemon under
# distinct network + container names.
# shellcheck source=../_lib/render-slice.sh
source "$ROOT/e2e/hurl/_lib/render-slice.sh"
render_slice rag-orchestrator

USER_ID_A="$(tr -d '\n' < "$ROOT/e2e/fixtures/rag-orchestrator/test-user-id.txt")"
USER_ID_EMPTY="$(tr -d '\n' < "$ROOT/e2e/fixtures/rag-orchestrator/test-empty-user-id.txt")"
CONV_ID="$(tr -d '\n' < "$ROOT/e2e/fixtures/rag-orchestrator/test-conversation-id.txt")"

REPORT_DIR="$ROOT/e2e/reports/rag-orchestrator-$RUN_ID"
mkdir -p "$REPORT_DIR"

cleanup() {
  if [[ "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> tearing down $STAGING_PROJECT_NAME stack" >&2
    # Tear down by project name only. Passing -f "$SLICE" here has been
    # observed to silently no-op under docker compose v29 on some hosts
    # — the project is already known to the daemon by name, so the flag
    # isn't needed for `down`, and dropping it avoids the silent skip.
    docker compose -p "$STAGING_PROJECT_NAME" \
      down -v --remove-orphans || true
  else
    echo "==> KEEP_STACK=1 — leaving $STAGING_PROJECT_NAME stack up" >&2
  fi
  # $SLICE_DIR is under mktemp -d; always clean up even when KEEP_STACK=1
  # so the resolved compose config (which could bake future ${VAR:-...}
  # secrets) doesn't linger in /tmp.
  rm -rf "$SLICE_DIR"
}
trap cleanup EXIT

echo "==> bringing up rag-orchestrator slice ($STAGING_PROJECT_NAME)" >&2
# --build is required because rag-orchestrator is a local build context
# in CI (no GHCR image pulled). --wait blocks on healthcheck convergence;
# rag-db-migrator's `restart: "no"` + service_completed_successfully gate
# guarantees migrations apply before rag-orchestrator starts.
docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
  up -d --wait --build \
  rag-db rag-db-migrator rag-orchestrator

echo "==> seeding augur conversation fixtures via psql" >&2
# `exec -T` disables TTY allocation so the SQL file can be piped on
# stdin. -v passes UUIDs into the SQL as :'name'::uuid placeholders so
# the fixture stays declarative.
docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" exec -T \
  -e PGPASSWORD=alt-staging-test-rag-password \
  rag-db psql -v ON_ERROR_STOP=1 \
    -U rag_user -d rag_db \
    -v user_id="$USER_ID_A" \
    -v conv_id="$CONV_ID" \
  < "$ROOT/e2e/fixtures/rag-orchestrator/db-seed.sql"

# Run Hurl inside the staging network so rag-orchestrator DNS resolves.
# Mount the repo at the same absolute path so any `file,e2e/fixtures/...;`
# body resolves via --file-root "$ROOT" — no such bodies today, but keep
# the mount consistent with the auth-hub / knowledge-sovereign runners.
hurl_run() {
  docker run --rm \
    --network "$STAGING_PROJECT_NAME" \
    -v "$ROOT:$ROOT" \
    -w "$ROOT" \
    "$HURL_IMAGE" \
    "$@"
}

common_vars=(
  --variable "base_url=$BASE_URL"
  --variable "connect_url=$CONNECT_URL"
  --variable "user_id=$USER_ID_A"
  --variable "empty_user_id=$USER_ID_EMPTY"
  --variable "conversation_id=$CONV_ID"
  --variable "run_id=$RUN_ID"
)

# nullglob so future 1[0-9]-*.hurl additions land without a script edit.
shopt -s nullglob
suite=(
  e2e/hurl/rag-orchestrator/0[0-9]-*.hurl
  e2e/hurl/rag-orchestrator/1[0-9]-*.hurl
)
shopt -u nullglob

echo "==> running Hurl suite (serial; 00-readiness waits for /readyz)" >&2
# --jobs 1 keeps ordering deterministic. 00-readiness' retry block waits
# out DB migrator completion, so no wrapper sleep is needed.
hurl_run --test \
  --jobs 1 \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  "${suite[@]}"

echo "==> suite passed. reports: $REPORT_DIR" >&2
