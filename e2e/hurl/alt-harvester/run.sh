#!/usr/bin/env bash
# e2e/hurl/alt-harvester/run.sh
#
# Brings up the alt-harvester slice of the alt-staging stack (Postgres +
# Atlas migrator + alt-backend-deps-stub + alt-harvester), runs the Hurl
# suite inside the alt-staging network, and tears the stack down.
#
# alt-harvester is the job-runner third of the alt-backend split. It owns
# the schedules that job.RegisterAllJobs wired into the monolith and serves
# no API; the only socket it opens is the operator listener on :9110
# (/health for compose, /metrics for Prometheus), and that is not published
# to the host. The suite is therefore mostly negative — see
# 01-operator-surface-only.hurl for what that buys.
#
# Environment overrides:
#   OPS_URL                — alt-harvester operator listener as seen from the
#                            Hurl container (default: http://alt-harvester:9110)
#   HURL_IMAGE             — Hurl container image (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   IMAGE_TAG              — Docker tag for the alt-harvester image (default: ci)
#   GHCR_OWNER             — GHCR namespace (default: kaikei-e)
#   RUN_ID                 — unique run identifier (default: $(date +%s))
#   STAGING_PROJECT_NAME   — compose project + network name (default: alt-staging).
#                            CI sets alt-staging-alt-harvester so parallel matrix
#                            jobs on the shared daemon don't collide.
#   KEEP_STACK=1           — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${OPS_URL:=http://alt-harvester:9110}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${IMAGE_TAG:=ci}"
: "${GHCR_OWNER:=kaikei-e}"
: "${RUN_ID:=$(date +%s)}"
: "${STAGING_PROJECT_NAME:=alt-staging}"

: "${ALT_HARVESTER_IMAGE_TAG:=$IMAGE_TAG}"
: "${ALT_BACKEND_DEPS_STUB_IMAGE_TAG:=$IMAGE_TAG}"
export IMAGE_TAG GHCR_OWNER STAGING_PROJECT_NAME ALT_HARVESTER_IMAGE_TAG ALT_BACKEND_DEPS_STUB_IMAGE_TAG

# shellcheck source=../_lib/render-slice.sh
source "$ROOT/e2e/hurl/_lib/render-slice.sh"
render_slice alt-harvester

# shellcheck source=../_lib/reclaim-network-pool.sh
source "$ROOT/e2e/hurl/_lib/reclaim-network-pool.sh"
reclaim_network_pool

# shellcheck source=../_lib/compose-up-with-retry.sh
source "$ROOT/e2e/hurl/_lib/compose-up-with-retry.sh"

# shellcheck source=../_lib/assert-transport-refused.sh
source "$ROOT/e2e/hurl/_lib/assert-transport-refused.sh"

REPORT_DIR="$ROOT/e2e/reports/alt-harvester-$RUN_ID"
mkdir -p "$REPORT_DIR"

cleanup() {
  local exit_code=$?
  if [[ "$exit_code" -ne 0 && "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> dumping $STAGING_PROJECT_NAME container logs (exit=$exit_code)" >&2
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
      logs --no-color --tail=500 >&2 2>&1 || true
  fi
  if [[ "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> tearing down $STAGING_PROJECT_NAME stack" >&2
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
      down -v --remove-orphans >/dev/null 2>&1 || true
  else
    echo "==> KEEP_STACK=1 — leaving $STAGING_PROJECT_NAME stack up" >&2
  fi
  rm -rf "$SLICE_DIR"
}
trap cleanup EXIT

echo "==> bringing up alt-harvester slice ($STAGING_PROJECT_NAME)" >&2
if ! compose_up_with_retry \
    alt-backend-db \
    alt-backend-db-migrator \
    alt-backend-deps-stub \
    alt-harvester; then
  echo "==> compose up failed — dumping logs before trap cleanup" >&2
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" ps -a >&2 2>&1 || true
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" logs --no-color --tail=500 >&2 2>&1 || true
  exit 1
fi

hurl_run() {
  docker run --rm \
    --network "$STAGING_PROJECT_NAME" \
    -v "$ROOT:$ROOT" \
    -w "$ROOT" \
    "$HURL_IMAGE" \
    "$@"
}

common_vars=(
  --variable "ops_url=$OPS_URL"
  --variable "run_id=$RUN_ID"
)

echo "==> running Hurl setup (serial; readiness probe)" >&2
hurl_run --test \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  e2e/hurl/alt-harvester/00-setup.hurl

echo "==> asserting the operator listener is the only socket" >&2
# 01-operator-surface-only.hurl covers the paths that must 404 on :9110.
# These are the ports that must not be bound at all: the two the monolith
# served the user API and RPC on, the loopback operator port alt-backend and
# alt-data-hub keep, and the TLS port that belongs to alt-data-hub. A 404
# from a bound port would still mean the harvester is listening for RPC.
for closed_port in 9000 9101 9102 9443; do
  assert_transport_refused "no listener on :$closed_port" \
    --file-root "$ROOT" \
    --variable "probe_url=http://alt-harvester:$closed_port/health" \
    e2e/hurl/_lib/probe-transport-refused.hurl
done

echo "==> running Hurl suite" >&2
# --jobs 1: the suite is read-only and short; serial keeps a broken run
# readable in order.
shopt -s nullglob
suite_files=(
  e2e/hurl/alt-harvester/0[1-9]-*.hurl
  e2e/hurl/alt-harvester/[1-9][0-9]-*.hurl
)
shopt -u nullglob

hurl_run --test \
  --jobs 1 \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  "${suite_files[@]}"

echo "==> suite passed. reports: $REPORT_DIR" >&2
