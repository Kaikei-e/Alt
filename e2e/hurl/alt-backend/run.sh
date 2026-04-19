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
#   BASE_URL               — alt-backend REST URL as seen from the Hurl container
#                            (default: http://alt-backend:9000)
#   CONNECT_URL            — alt-backend Connect-RPC URL (default: http://alt-backend:9101)
#   HURL_IMAGE             — Hurl container image (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   IMAGE_TAG              — Docker tag for the alt-backend image (default: ci)
#   GHCR_OWNER             — GHCR namespace (default: kaikei-e)
#   RUN_ID                 — unique run identifier (default: $(date +%s))
#   STAGING_PROJECT_NAME   — compose project + network name (default: alt-staging).
#                            CI sets alt-staging-alt-backend so parallel matrix
#                            jobs on koko-1 don't collide on the shared daemon.
#   KEEP_STACK=1           — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${BASE_URL:=http://alt-backend:9000}"
: "${CONNECT_URL:=http://alt-backend:9101}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${IMAGE_TAG:=ci}"
: "${GHCR_OWNER:=kaikei-e}"
: "${RUN_ID:=$(date +%s)}"
: "${STAGING_PROJECT_NAME:=alt-staging}"

export IMAGE_TAG GHCR_OWNER STAGING_PROJECT_NAME

# Render a per-project compose slice (sets $SLICE + $SLICE_DIR). This
# lets parallel matrix jobs coexist on the same Docker daemon by
# renaming network + container resources under STAGING_PROJECT_NAME.
# shellcheck source=../_lib/render-slice.sh
source "$ROOT/e2e/hurl/_lib/render-slice.sh"
render_slice alt-backend

REPORT_DIR="$ROOT/e2e/reports/alt-backend-$RUN_ID"
mkdir -p "$REPORT_DIR"

cleanup() {
  if [[ "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> tearing down $STAGING_PROJECT_NAME stack" >&2
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
      down -v --remove-orphans >/dev/null 2>&1 || true
  else
    echo "==> KEEP_STACK=1 — leaving $STAGING_PROJECT_NAME stack up" >&2
  fi
  # $SLICE_DIR is under mktemp -d; always clean up, even when
  # KEEP_STACK=1, so resolved compose config doesn't linger.
  rm -rf "$SLICE_DIR"
}
trap cleanup EXIT

# Read the pre-minted test JWT from the fixture and strip any trailing
# newline — HTTP header values must not contain CR/LF.
JWT="$(tr -d '\n' < e2e/fixtures/alt-backend/test-jwt.txt)"

echo "==> bringing up alt-backend slice ($STAGING_PROJECT_NAME)" >&2
docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" up -d --wait \
    alt-backend-db \
    alt-backend-db-migrator \
    alt-backend-deps-stub \
    alt-backend

# Run Hurl inside the staging network so alt-backend's service DNS
# name resolves. Mount the repo at the same absolute path so any
# `file,e2e/fixtures/...;` body resolves via --file-root "$ROOT".
hurl_run() {
  docker run --rm \
    --network "$STAGING_PROJECT_NAME" \
    -v "$ROOT:$ROOT" \
    -w "$ROOT" \
    "$HURL_IMAGE" \
    "$@"
}

# Credentials flow through --secret so Hurl redacts them from
# --report-html / --report-junit (audit F-002). Non-sensitive values
# stay on --variable.
common_vars=(
  --variable "base_url=$BASE_URL"
  --variable "connect_url=$CONNECT_URL"
  --variable "run_id=$RUN_ID"
  --secret   "jwt=$JWT"
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
