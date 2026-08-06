#!/usr/bin/env bash
# e2e/playwright/alt-backend/run.sh
#
# Brings up the alt-backend slice of the alt-staging stack (Postgres + Atlas
# migrator + alt-backend-deps-stub + alt-data-hub + alt-backend), runs the
# Playwright API suite inside the alt-staging network so the alt-backend DNS
# name resolves, and tears the stack down.
#
# This replaced e2e/hurl/alt-backend/ (ADR-000766 established the
# `e2e/<framework>/<svc>/run.sh` dispatch contract; alt-deploy's
# release-deploy.yaml probes both directories). The compose lifecycle below is
# a straight port of the Hurl runner's — same slice, same PKI minting, same
# retry, same log dumping and redaction — so the only thing that changed is
# what runs against the stack.
#
# Why two containers
# ------------------
# The staging network is `internal: true`: containers on it have no route to
# the Internet, and host port publishes are silently ignored. So the suite has
# to run *inside* the network to reach alt-backend, but `npm ci` has to run
# *outside* it to reach the registry. Step 1 installs into the bind-mounted
# workspace on the default bridge; step 2 runs the tests on the staging network
# with those node_modules already in place.
#
# Why a plain node image and not mcr.microsoft.com/playwright
# -----------------------------------------------------------
# No spec in this suite touches `page`, `browser` or `context` — it is HTTP
# only — so Playwright never launches a browser and the browser binaries are
# dead weight. A node image plus PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 is ~150MB
# against the Playwright image's ~1.7GB, which is most of this suite's
# cold-start cost.
#
# Environment overrides:
#   BASE_URL               — alt-backend REST URL as seen from the test container
#                            (default: http://alt-backend:9000)
#   CONNECT_URL            — user-facing Connect-RPC URL (default: http://alt-backend:9101)
#   INTERNAL_URL           — loopback operator Connect-RPC URL (default: http://alt-backend:9102)
#   OPS_URL                — operator listener, /health + /metrics
#                            (default: http://alt-backend:9110). The three split
#                            binaries share this listener; Prometheus scrapes all
#                            of them there.
#   NODE_IMAGE             — container image for install + run (default: node:22-bookworm-slim)
#   SHARD                  — Playwright --shard value (default: unset = whole suite)
#   PW_WORKERS             — worker count inside the shard (default: 6, see playwright.config.ts)
#   PW_BLOB                — 1 to emit a blob report for merge-reports (CI matrix sets this)
#   PW_GREP                — value for --grep, to run a subset
#   IMAGE_TAG              — Docker tag for the alt-backend image (default: ci)
#   GHCR_OWNER             — GHCR namespace (default: kaikei-e)
#   RUN_ID                 — unique run identifier (default: $(date +%s))
#   STAGING_PROJECT_NAME   — compose project + network name (default: alt-staging).
#                            CI sets alt-staging-alt-backend so parallel matrix
#                            jobs on a shared daemon don't collide.
#   KEEP_STACK=1           — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
SUITE_DIR="$ROOT/e2e/playwright/alt-backend"
cd "$ROOT"

: "${BASE_URL:=http://alt-backend:9000}"
: "${CONNECT_URL:=http://alt-backend:9101}"
: "${INTERNAL_URL:=http://alt-backend:9102}"
: "${OPS_URL:=http://alt-backend:9110}"
: "${NODE_IMAGE:=node:22-bookworm-slim}"
: "${IMAGE_TAG:=ci}"
: "${GHCR_OWNER:=kaikei-e}"
: "${RUN_ID:=$(date +%s)}"
: "${STAGING_PROJECT_NAME:=alt-staging}"

# Per-service image tag: compose.staging.yaml keys each GHCR image off its own
# `<SERVICE>_IMAGE_TAG` env var (default `main`) so unrelated dependency
# services stay on the last successful main build even when the dispatch SHA
# has no rebuild for them. This suite forwards its IMAGE_TAG into its
# service-scoped vars; backend-deps-stub and alt-data-hub are co-versioned with
# alt-backend because they share the same build stream / Go module.
: "${ALT_BACKEND_IMAGE_TAG:=$IMAGE_TAG}"
: "${ALT_BACKEND_DEPS_STUB_IMAGE_TAG:=$IMAGE_TAG}"
: "${ALT_DATA_HUB_IMAGE_TAG:=$IMAGE_TAG}"

# Throwaway mTLS material for the alt-backend → alt-data-hub hop. It has to sit
# under e2e/fixtures/ so compose can bind-mount it, which also puts private keys
# one `git add e2e/` away from a commit — hence the entry in e2e/.gitignore.
# STAGING_PKI_DIR is what compose.staging.yaml mounts at /certs and /trust; it
# is absolute and suite-scoped so parallel matrix jobs never share a directory.
PKI_DIR="$ROOT/e2e/fixtures/alt-backend/pki"
STAGING_PKI_DIR="$PKI_DIR"

export IMAGE_TAG GHCR_OWNER STAGING_PROJECT_NAME ALT_BACKEND_IMAGE_TAG \
       ALT_BACKEND_DEPS_STUB_IMAGE_TAG ALT_DATA_HUB_IMAGE_TAG STAGING_PKI_DIR

# Render a per-project compose slice (sets $SLICE + $SLICE_DIR) so parallel
# matrix jobs can coexist on one Docker daemon. Shared with the Hurl suites —
# `_lib` is framework-agnostic despite living under e2e/hurl/.
# shellcheck source=../../hurl/_lib/render-slice.sh
source "$ROOT/e2e/hurl/_lib/render-slice.sh"
render_slice alt-backend

# shellcheck source=../../hurl/_lib/reclaim-network-pool.sh
source "$ROOT/e2e/hurl/_lib/reclaim-network-pool.sh"
reclaim_network_pool

# shellcheck source=../../hurl/_lib/compose-up-with-retry.sh
source "$ROOT/e2e/hurl/_lib/compose-up-with-retry.sh"

# shellcheck source=../../hurl/_lib/mint-staging-pki.sh
source "$ROOT/e2e/hurl/_lib/mint-staging-pki.sh"

REPORT_DIR="${REPORT_DIR:-$ROOT/e2e/reports/alt-backend-$RUN_ID}"
mkdir -p "$REPORT_DIR"

# Redact JWT-like compact-serialization strings from an input stream.
# Rationale (security audit F-002): `docker compose logs` dumps container
# stdout/stderr verbatim into the CI job output, and any error path that logs a
# request header can carry the staging test JWT. Redact before the bytes hit
# the job log, not after. Matches the 3-segment JWS compact form with the
# mandatory `eyJ` header prefix and the RFC 7515 base64url charset.
redact_secrets() {
  sed -E 's#eyJ[A-Za-z0-9_=-]+\.eyJ[A-Za-z0-9_=-]+\.[A-Za-z0-9_=-]+#[REDACTED_JWT]#g'
}

cleanup() {
  local exit_code=$?
  # Dump per-container logs to stderr BEFORE teardown when the suite failed.
  # The Ansible wrapper (playbooks/run-e2e-suite.yml) has its own dump task in
  # `always:`, but it races with this trap — by the time it runs, the stack is
  # gone and `docker compose logs` finds nothing.
  if [[ "$exit_code" -ne 0 && "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> dumping $STAGING_PROJECT_NAME container logs (exit=$exit_code)" >&2
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
      logs --no-color --tail=500 2>&1 | redact_secrets >&2 || true
  fi
  if [[ "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> tearing down $STAGING_PROJECT_NAME stack" >&2
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
      down -v --remove-orphans >/dev/null 2>&1 || true
    rm -rf "$PKI_DIR"
  else
    echo "==> KEEP_STACK=1 — leaving $STAGING_PROJECT_NAME stack up" >&2
    echo "==> KEEP_STACK=1 — leaving $PKI_DIR in place (mounted by the containers)" >&2
  fi
  # $SLICE_DIR is under mktemp -d; always clean up, even under KEEP_STACK=1, so
  # resolved compose config (which bakes in environment values) does not linger.
  rm -rf "$SLICE_DIR"
}
trap cleanup EXIT

# One CA, one server leaf for alt-data-hub and one client leaf named
# `alt-backend` — which is what DATAHUB_ALLOWED_PEERS admits. Minted before the
# stack comes up because di.newDataHubClient reads the CA at construction and
# panics if it cannot (CLAUDE.md rules 8/9).
mint_staging_pki "$PKI_DIR" alt-data-hub alt-backend

echo "==> installing suite dependencies (default bridge — the staging network has no egress)" >&2
docker run --rm \
  -v "$ROOT:$ROOT" \
  -w "$SUITE_DIR" \
  -e PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
  -e npm_config_update_notifier=false \
  "$NODE_IMAGE" \
  npm ci --no-audit --no-fund

echo "==> bringing up alt-backend slice ($STAGING_PROJECT_NAME)" >&2
if ! compose_up_with_retry \
    alt-backend-db \
    alt-backend-db-migrator \
    alt-backend-deps-stub \
    alt-data-hub \
    alt-backend; then
  # A dependency failure during `up --wait` (e.g. the stub exits 1 on import)
  # makes compose *immediately* tear the failing container down, so the EXIT
  # trap's `docker compose logs` finds nothing. Grab them here, while the
  # containers are still addressable by project name.
  echo "==> compose up failed — dumping logs before trap cleanup" >&2
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" ps -a >&2 2>&1 || true
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" logs --no-color --tail=500 2>&1 \
    | redact_secrets >&2 || true
  exit 1
fi

# --shard is passed through only when set: Playwright rejects an empty value,
# and `1/1` is not the same as omitting it for report merging.
playwright_args=()
if [[ -n "${SHARD:-}" ]]; then
  if ! [[ "$SHARD" =~ ^[0-9]+/[0-9]+$ ]]; then
    echo "SHARD must match N/M (got: $SHARD)" >&2
    exit 2
  fi
  playwright_args+=("--shard=$SHARD")
fi
if [[ -n "${PW_GREP:-}" ]]; then
  playwright_args+=("--grep" "$PW_GREP")
fi

echo "==> running Playwright API suite${SHARD:+ (shard $SHARD)}" >&2
docker run --rm \
  --network "$STAGING_PROJECT_NAME" \
  -v "$ROOT:$ROOT" \
  -w "$SUITE_DIR" \
  -e CI="${CI:-}" \
  -e FORCE_COLOR=1 \
  -e BASE_URL="$BASE_URL" \
  -e CONNECT_URL="$CONNECT_URL" \
  -e INTERNAL_URL="$INTERNAL_URL" \
  -e OPS_URL="$OPS_URL" \
  -e JWT_FILE="$ROOT/e2e/fixtures/alt-backend/test-jwt.txt" \
  -e RUN_ID="$RUN_ID" \
  -e ARTICLE_ID_SEED="${ARTICLE_ID_SEED:-00000000-0000-0000-0000-000000000001}" \
  -e PW_REPORT_DIR="$REPORT_DIR" \
  -e PW_BLOB="${PW_BLOB:-}" \
  -e PW_WORKERS="${PW_WORKERS:-}" \
  "$NODE_IMAGE" \
  npx playwright test "${playwright_args[@]}"

echo "==> suite passed. reports: $REPORT_DIR" >&2
