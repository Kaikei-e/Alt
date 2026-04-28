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

# Per-service image tag: compose.staging.yaml keys each GHCR image off
# its own `<SERVICE>_IMAGE_TAG` env var (default `main`) so unrelated
# dependency services stay on the last successful main build even when
# the dispatch SHA has no rebuild for them. The alt-backend suite
# forwards its IMAGE_TAG into its service-scoped var; backend-deps-stub
# is co-versioned with alt-backend because they share the same build
# stream.
: "${ALT_BACKEND_IMAGE_TAG:=$IMAGE_TAG}"
: "${ALT_BACKEND_DEPS_STUB_IMAGE_TAG:=$IMAGE_TAG}"
export IMAGE_TAG GHCR_OWNER STAGING_PROJECT_NAME ALT_BACKEND_IMAGE_TAG ALT_BACKEND_DEPS_STUB_IMAGE_TAG

# Render a per-project compose slice (sets $SLICE + $SLICE_DIR). This
# lets parallel matrix jobs coexist on the same Docker daemon by
# renaming network + container resources under STAGING_PROJECT_NAME.
# shellcheck source=../_lib/render-slice.sh
source "$ROOT/e2e/hurl/_lib/render-slice.sh"
render_slice alt-backend

# Pre-cleanup: reclaim Docker's pre-defined address pool from networks
# left by cancelled prior runs. Safe-by-default: docker network prune
# refuses to touch networks an active container is attached to.
# shellcheck source=../_lib/reclaim-network-pool.sh
source "$ROOT/e2e/hurl/_lib/reclaim-network-pool.sh"
reclaim_network_pool

REPORT_DIR="$ROOT/e2e/reports/alt-backend-$RUN_ID"
mkdir -p "$REPORT_DIR"

# Redact JWT-like compact-serialization strings from an input stream.
# Rationale (security audit F-002 on 2026-04-24): `docker compose logs` dumps
# alt-backend container stdout/stderr verbatim into the CI job output. Any
# error path that logs a request header can carry the staging test JWT
# (still mint-able, but also a password in fixture scope). alt-deploy's job
# logs are private today, but defense-in-depth demands we redact before the
# bytes hit the job log — not after. Matches the 3-segment JWS compact
# form with the mandatory `eyJ` header prefix and RFC 7515 base64url charset.
redact_secrets() {
  sed -E 's#eyJ[A-Za-z0-9_=-]+\.eyJ[A-Za-z0-9_=-]+\.[A-Za-z0-9_=-]+#[REDACTED_JWT]#g'
}

cleanup() {
  local exit_code=$?
  # Dump per-container logs to stderr BEFORE teardown when Hurl failed.
  # The Ansible wrapper (playbooks/run-e2e-suite.yml) has its own log dump
  # task in `always:`, but it races with this `trap EXIT` — by the time the
  # playbook's task runs, this trap has already torn the stack down and
  # `docker compose logs` finds nothing. Dumping inline here is the only
  # reliable path to capture alt-backend's ERROR-level log lines when a
  # Connect-RPC handler fails (e.g. transition_knowledge_loop failed).
  if [[ "$exit_code" -ne 0 && "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> dumping $STAGING_PROJECT_NAME container logs (exit=$exit_code)" >&2
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
      logs --no-color --tail=500 2>&1 | redact_secrets >&2 || true
  fi
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
if ! docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" up -d --wait \
    alt-backend-db \
    alt-backend-db-migrator \
    alt-backend-deps-stub \
    alt-backend; then
  # A dependency failure during `up --wait` (e.g. stub exits 1 on import)
  # makes compose *immediately* tear the failing container down. By the
  # time the EXIT trap runs `docker compose logs`, the failing container
  # is already gone and the output is empty. Grab logs right here —
  # before we let the error propagate — while the containers (or at
  # least their log streams) are still addressable by the project name.
  echo "==> compose up failed — dumping logs before trap cleanup" >&2
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" ps -a >&2 2>&1 || true
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" logs --no-color --tail=500 2>&1 \
    | redact_secrets >&2 || true
  exit 1
fi

# Fresh UUIDv7 values for the Knowledge Loop transition suite.
# The backend validator (ValidateClientTransitionID) enforces UUIDv7 format
# *and* a −48h / +5m embedded-timestamp window, so these must be minted at
# run time rather than pinned as literals. python3 is already required by
# the ansible runner, so we lean on it instead of adding another dep.
uuidv7() {
  python3 - <<'PY'
import os, time
ms = int(time.time() * 1000)
rb = os.urandom(10)
b = ms.to_bytes(6, 'big')
vr = 0x7000 | (rb[0] << 4) | (rb[1] >> 4)
b += vr.to_bytes(2, 'big')
var = 0x8000000000000000 | (int.from_bytes(rb[2:10], 'big') & 0x3FFFFFFFFFFFFFFF)
b += var.to_bytes(8, 'big')
h = b.hex()
print(f'{h[0:8]}-{h[8:12]}-{h[12:16]}-{h[16:20]}-{h[20:32]}')
PY
}
LOOP_TX_FORBIDDEN="$(uuidv7)"
LOOP_TX_ACCEPT="$(uuidv7)"
LOOP_TX_MALFORMED="$(uuidv7)"
LOOP_TX_OBS_DECIDE="$(uuidv7)"
LOOP_TX_ORIENT_DECIDE="$(uuidv7)"
LOOP_TX_DECIDE_ACT="$(uuidv7)"
LOOP_TX_ACT_OBSERVE="$(uuidv7)"
LOOP_TX_ORIENT_ACT="$(uuidv7)"
LOOP_TX_DECIDE_OBSERVE="$(uuidv7)"
LOOP_TX_ACT_ACT="$(uuidv7)"
LOOP_TX_REVIEW_RECHECK="$(uuidv7)"
LOOP_TX_REVIEW_ARCHIVE="$(uuidv7)"
LOOP_TX_REVIEW_MARK_REVIEWED="$(uuidv7)"
LOOP_TX_REVIEW_CROSS_STAGE="$(uuidv7)"

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
  --variable "loop_tx_forbidden=$LOOP_TX_FORBIDDEN"
  --variable "loop_tx_accept=$LOOP_TX_ACCEPT"
  --variable "loop_tx_malformed=$LOOP_TX_MALFORMED"
  --variable "loop_tx_obs_decide=$LOOP_TX_OBS_DECIDE"
  --variable "loop_tx_orient_decide=$LOOP_TX_ORIENT_DECIDE"
  --variable "loop_tx_decide_act=$LOOP_TX_DECIDE_ACT"
  --variable "loop_tx_act_observe=$LOOP_TX_ACT_OBSERVE"
  --variable "loop_tx_orient_act=$LOOP_TX_ORIENT_ACT"
  --variable "loop_tx_decide_observe=$LOOP_TX_DECIDE_OBSERVE"
  --variable "loop_tx_act_act=$LOOP_TX_ACT_ACT"
  --variable "loop_tx_review_recheck=$LOOP_TX_REVIEW_RECHECK"
  --variable "loop_tx_review_archive=$LOOP_TX_REVIEW_ARCHIVE"
  --variable "loop_tx_review_mark_reviewed=$LOOP_TX_REVIEW_MARK_REVIEWED"
  --variable "loop_tx_review_cross_stage=$LOOP_TX_REVIEW_CROSS_STAGE"
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
