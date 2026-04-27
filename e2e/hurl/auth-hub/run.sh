#!/usr/bin/env bash
# e2e/hurl/auth-hub/run.sh
#
# Brings up the auth-hub slice of the alt-staging stack (kratos-db
# Postgres + Kratos migrator + Kratos + auth-hub), seeds an identity via
# Kratos Admin API, captures a session via the self-service api-flow
# login, and runs the Hurl suite inside the alt-staging network so every
# service DNS name resolves.
#
# The staging network is `internal: true`, which silently ignores host
# port publishes. Running Hurl inside the network is the only portable
# way to reach the SUT, matching the acolyte-orchestrator /
# knowledge-sovereign / mq-hub pattern.
#
# Captures are file-scoped in Hurl `--test` mode, so this script runs
# 00-setup as a separate invocation with `--report-json`, parses the
# session_token / user_id / session_id captures out of the report, and
# injects them as --variable inputs into the main suite run.
#
# Environment overrides:
#   BASE_URL              — auth-hub REST URL as seen from the Hurl
#                           container (default: http://auth-hub:8888)
#   KRATOS_PUBLIC_URL     — Kratos FrontendAPI URL (default: http://kratos:4433)
#   KRATOS_ADMIN_URL      — Kratos AdminAPI URL (default: http://kratos:4434)
#   HURL_IMAGE            — Hurl container image
#                           (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   IMAGE_TAG             — Docker tag for the auth-hub image (default: ci)
#   GHCR_OWNER            — GHCR namespace (default: kaikei-e)
#   RUN_ID                — unique run identifier (default: $(date +%s))
#   STAGING_PROJECT_NAME  — compose project + network name (default: alt-staging).
#                           CI sets alt-staging-auth-hub so parallel matrix
#                           jobs on the self-hosted deploy runner don't
#                           collide on the shared Docker daemon.
#   KEEP_STACK=1          — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${BASE_URL:=http://auth-hub:8888}"
: "${KRATOS_PUBLIC_URL:=http://kratos:4433}"
: "${KRATOS_ADMIN_URL:=http://kratos:4434}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${IMAGE_TAG:=ci}"
: "${GHCR_OWNER:=kaikei-e}"
: "${RUN_ID:=$(date +%s)}"
: "${STAGING_PROJECT_NAME:=alt-staging}"

# Per-service image tag: see search-indexer/run.sh for rationale. This
# suite's focal service is auth-hub.
: "${AUTH_HUB_IMAGE_TAG:=$IMAGE_TAG}"
export IMAGE_TAG GHCR_OWNER STAGING_PROJECT_NAME AUTH_HUB_IMAGE_TAG

# Render a per-project compose slice (sets $SLICE + $SLICE_DIR). This
# lets parallel matrix jobs coexist on the same Docker daemon by
# renaming network + container resources under STAGING_PROJECT_NAME.
# shellcheck source=../_lib/render-slice.sh
source "$ROOT/e2e/hurl/_lib/render-slice.sh"
render_slice auth-hub

# Pre-cleanup: reclaim Docker's pre-defined address pool from networks
# left by cancelled prior runs. Safe-by-default: docker network prune
# refuses to touch networks an active container is attached to.
# shellcheck source=../_lib/reclaim-network-pool.sh
source "$ROOT/e2e/hurl/_lib/reclaim-network-pool.sh"
reclaim_network_pool

# Read fixtures. Secrets go through --secret so Hurl redacts them from
# --report-html / --report-junit (audit F-002); non-sensitive values
# ride --variable.
BACKEND_TOKEN_SECRET="$(tr -d '\n' < "$ROOT/e2e/fixtures/staging-secrets/alt_backend_token_secret.txt")"
IDENTITY_EMAIL="$(tr -d '\n' < "$ROOT/e2e/fixtures/auth-hub/test-identity-email.txt")"
IDENTITY_PASSWORD="$(tr -d '\n' < "$ROOT/e2e/fixtures/auth-hub/test-identity-password.txt")"
TENANT_ID="$(tr -d '\n' < "$ROOT/e2e/fixtures/auth-hub/test-tenant-id.txt")"

REPORT_DIR="$ROOT/e2e/reports/auth-hub-$RUN_ID"
SEED_REPORT_DIR="$REPORT_DIR/seed-json"
mkdir -p "$REPORT_DIR" "$SEED_REPORT_DIR"

cleanup() {
  if [[ "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> tearing down $STAGING_PROJECT_NAME stack" >&2
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
      down -v --remove-orphans >/dev/null 2>&1 || true
  else
    echo "==> KEEP_STACK=1 — leaving $STAGING_PROJECT_NAME stack up" >&2
  fi
  # $SLICE_DIR is under mktemp -d; always clean up, even when
  # KEEP_STACK=1, so the resolved compose config doesn't linger.
  rm -rf "$SLICE_DIR"
}
trap cleanup EXIT

echo "==> bringing up auth-hub slice ($STAGING_PROJECT_NAME)" >&2
# --build is required because auth-hub is a local build context (no GHCR
# image pulled in CI). --wait blocks on healthcheck convergence; the
# migrator's restart=no + kratos's service_completed_successfully gate
# guarantees `kratos migrate sql` runs before Kratos proper starts, and
# Kratos's healthcheck is the gate on auth-hub.
docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
  up -d --wait --build \
  auth-hub-db auth-hub-db-migrator kratos auth-hub

# Run Hurl inside the staging network so auth-hub / kratos DNS names
# resolve. Mount the repo at the same absolute path so any
# `file,e2e/fixtures/...;` body resolves via --file-root "$ROOT".
hurl_run() {
  docker run --rm \
    --network "$STAGING_PROJECT_NAME" \
    -v "$ROOT:$ROOT" \
    -w "$ROOT" \
    "$HURL_IMAGE" \
    "$@"
}

seed_vars=(
  --variable "auth_hub_url=$BASE_URL"
  --variable "kratos_public_url=$KRATOS_PUBLIC_URL"
  --variable "kratos_admin_url=$KRATOS_ADMIN_URL"
  --variable "email=$IDENTITY_EMAIL"
  --variable "tenant_id=$TENANT_ID"
  --variable "run_id=$RUN_ID"
  --secret   "password=$IDENTITY_PASSWORD"
)

echo "==> running Hurl setup (identity seed + api-flow login)" >&2
# Standalone invocation so captures can be extracted from the JSON
# report. --no-output suppresses response bodies on stdout.
hurl_run --test \
  --no-output \
  --report-json "$SEED_REPORT_DIR" \
  --file-root "$ROOT" \
  "${seed_vars[@]}" \
  e2e/hurl/auth-hub/00-setup.hurl

# Extract captures from the seed report. jq is pre-installed on the
# self-hosted deploy runner and on typical dev boxes; using host jq
# keeps the dependency surface minimal.
capture_value() {
  local name="$1"
  jq -r --arg n "$name" \
    '.[0].entries[] | select(.captures != null) | .captures[] | select(.name==$n) | .value' \
    "$SEED_REPORT_DIR/report.json" | tail -1
}

KRATOS_SESSION_COOKIE="$(capture_value kratos_session_cookie)"
USER_ID="$(capture_value user_id)"
SESSION_ID="$(capture_value session_id)"

if [[ -z "$KRATOS_SESSION_COOKIE" || -z "$USER_ID" || -z "$SESSION_ID" ]]; then
  echo "ERROR: 00-setup.hurl did not produce the expected captures" >&2
  echo "  kratos_session_cookie=<${#KRATOS_SESSION_COOKIE} bytes>" >&2
  echo "  user_id='$USER_ID' session_id='$SESSION_ID'" >&2
  exit 1
fi

echo "==> seed captures: user_id=$USER_ID session_id=$SESSION_ID" >&2

common_vars=(
  --variable "auth_hub_url=$BASE_URL"
  --variable "kratos_public_url=$KRATOS_PUBLIC_URL"
  --variable "kratos_admin_url=$KRATOS_ADMIN_URL"
  --variable "email=$IDENTITY_EMAIL"
  --variable "tenant_id=$TENANT_ID"
  --variable "user_id=$USER_ID"
  --variable "session_id=$SESSION_ID"
  --variable "run_id=$RUN_ID"
  --secret   "kratos_session_cookie=$KRATOS_SESSION_COOKIE"
  --secret   "password=$IDENTITY_PASSWORD"
  --secret   "backend_token_secret=$BACKEND_TOKEN_SECRET"
)

# Collect suite files via nullglob so future increments can land
# 1[3-9]-*.hurl / 2[0-9]-*.hurl without script edits.
shopt -s nullglob
suite_files=(
  e2e/hurl/auth-hub/0[1-9]-*.hurl
  e2e/hurl/auth-hub/1[0-2]-*.hurl
)
shopt -u nullglob

echo "==> running Hurl suite (serial; captures from 00-setup injected as --variable)" >&2
# --jobs 1 keeps /internal/system-user scenarios sequential — internal
# rate limit is 10 req/min burst 3 and is not env-tunable. Per-scenario
# [Options] retry intervals on 09/10/11 wait out the limiter.
hurl_run --test \
  --jobs 1 \
  --retry 5 \
  --retry-interval 500 \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  "${suite_files[@]}"

echo "==> suite passed. reports: $REPORT_DIR" >&2
