#!/usr/bin/env bash
# e2e/hurl/alt-backend/run.sh
#
# Brings up the alt-backend slice of the alt-staging stack (Postgres +
# Atlas migrator + alt-backend-deps-stub + alt-backend), runs the Hurl
# suite inside the alt-staging network so the alt-backend DNS name
# resolves, and tears the stack down.
#
# The staging network is `internal: true`, which silently ignores host
# port publishes. Running Hurl inside the network is the only portable
# way to reach the SUT, and matches the mq-hub / knowledge-sovereign /
# recap-worker pattern.
#
# Environment overrides:
#   BASE_URL        — alt-backend REST URL as seen from the Hurl container
#                     (default: http://alt-backend:9000)
#   CONNECT_URL     — alt-backend Connect-RPC URL (default: http://alt-backend:9101)
#   HURL_IMAGE      — Hurl container image (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   IMAGE_TAG       — Docker tag for the alt-backend image (default: ci)
#   GHCR_OWNER      — GHCR namespace (default: kaikei-e)
#   RUN_ID          — unique run identifier (default: $(date +%s))
#   KEEP_STACK=1    — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${BASE_URL:=http://alt-backend:9000}"
: "${CONNECT_URL:=http://alt-backend:9101}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${IMAGE_TAG:=ci}"
: "${GHCR_OWNER:=kaikei-e}"
: "${RUN_ID:=$(date +%s)}"

export IMAGE_TAG GHCR_OWNER

REPORT_DIR="$ROOT/e2e/reports/alt-backend-$RUN_ID"
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

# Read the pre-minted test JWT from the fixture and strip any trailing
# newline — HTTP header values must not contain CR/LF.
JWT="$(tr -d '\n' < e2e/fixtures/alt-backend/test-jwt.txt)"

echo "==> bringing up alt-backend staging slice" >&2
docker compose -f compose/compose.staging.yaml -p alt-staging \
  --profile alt-backend \
  up -d --wait \
    alt-backend-db \
    alt-backend-db-migrator \
    alt-backend-deps-stub \
    alt-backend

# Run Hurl inside the alt-staging network so alt-backend's service DNS
# name resolves. Mount the repo at the same absolute path so any
# `file,e2e/fixtures/...;` body resolves via --file-root "$ROOT".
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
  --variable "connect_url=$CONNECT_URL"
  --variable "jwt=$JWT"
  --variable "run_id=$RUN_ID"
)

echo "==> running Hurl setup (serial; readiness probe)" >&2
hurl_run --test \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  e2e/hurl/alt-backend/00-setup.hurl

echo "==> running Hurl suite" >&2
# --jobs 4 is safe: every state-mutating scenario is self-contained
# (captures its own csrf token and operates on its own feed URLs).
# --retry 5 covers transient 5xx from the stub during cold-start.
hurl_run --test \
  --jobs 4 \
  --retry 5 \
  --retry-interval 500 \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  e2e/hurl/alt-backend/0[1-9]-*.hurl \
  e2e/hurl/alt-backend/[1-9][0-9]-*.hurl

echo "==> suite passed. reports: $REPORT_DIR" >&2
