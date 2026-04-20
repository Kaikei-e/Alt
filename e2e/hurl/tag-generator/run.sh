#!/usr/bin/env bash
# e2e/hurl/tag-generator/run.sh
#
# Brings up the tag-generator slice of the alt-staging stack
# (redis-streams + stub-backend + mq-hub + tag-generator), runs the Hurl
# suite inside the alt-staging network (Hurl's Docker image joins the
# network so the `tag-generator` and `mq-hub` service DNS resolves),
# and tears down.
#
# The alt-staging network is `internal: true`, which silently ignores
# host port publishes. Running Hurl inside the network is the only
# portable way to reach the SUT, matching search-indexer / mq-hub /
# knowledge-sovereign.
#
# Scenario 04 round-trips through mq-hub's GenerateTagsForArticle RPC
# because Hurl cannot XREAD Redis directly; mq-hub therefore has to be
# in the same staging slice as tag-generator.
#
# Environment overrides:
#   BASE_URL        — tag-generator URL as seen from the Hurl container
#                     (default: http://tag-generator:9400)
#   MQHUB_BASE_URL  — mq-hub Connect-RPC URL for scenario 04
#                     (default: http://mq-hub:9500)
#   HURL_IMAGE      — Hurl container image (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   IMAGE_TAG       — tag of ghcr.io/<owner>/alt-tag-generator + alt-mq-hub
#                     (default: main)
#   GHCR_OWNER      — GHCR namespace (default: kaikei-e)
#   RUN_ID               — unique run identifier for articleId isolation
#                          (default: $(date +%s))
#   STAGING_PROJECT_NAME — compose project + network name (default: alt-staging).
#                          CI sets alt-staging-tag-generator so parallel matrix
#                          jobs on the shared Docker daemon don't collide.
#   KEEP_STACK=1         — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${IMAGE_TAG:=main}"
: "${GHCR_OWNER:=kaikei-e}"
: "${BASE_URL:=http://tag-generator:9400}"
: "${MQHUB_BASE_URL:=http://mq-hub:9500}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${RUN_ID:=$(date +%s)}"
: "${STAGING_PROJECT_NAME:=alt-staging}"

# Per-service image tag: see search-indexer/run.sh for rationale.
: "${TAG_GENERATOR_IMAGE_TAG:=$IMAGE_TAG}"
export IMAGE_TAG GHCR_OWNER STAGING_PROJECT_NAME TAG_GENERATOR_IMAGE_TAG

# shellcheck source=../_lib/render-slice.sh
source "$ROOT/e2e/hurl/_lib/render-slice.sh"
render_slice tag-generator

REPORT_DIR="$ROOT/e2e/reports/tag-generator-$RUN_ID"
mkdir -p "$REPORT_DIR"

cleanup() {
  local rc=$?
  # On failure: dump container logs before teardown so the Python
  # traceback (SBERT / MeCab / unidic init path) is visible in CI.
  # Without this, runtime errors surface as silent 503 retry loops —
  # /api/v1/extract-tags returns 503 for the whole Hurl budget while
  # the container's stderr (where the real exception lives) never
  # reaches the job log. Cheap on success, critical on failure.
  if [[ "$rc" -ne 0 ]]; then
    echo "==> Hurl suite failed (rc=$rc). Dumping container logs:" >&2
    for svc in tag-generator mq-hub stub-backend redis-streams; do
      echo "---- $svc ----" >&2
      docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
        logs --tail=200 --no-color "$svc" 2>&1 | sed "s/^/[$svc] /" >&2 || true
    done
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

echo "==> bringing up tag-generator slice ($STAGING_PROJECT_NAME)" >&2
docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
  up -d --wait redis-streams stub-backend mq-hub tag-generator

# Run Hurl inside the staging network so in-network DNS resolves both
# the tag-generator and mq-hub services. Mount the repo at the same
# absolute path so any `file,e2e/fixtures/...;` body resolves via
# --file-root "$ROOT".
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
  --variable "mqhub_base_url=$MQHUB_BASE_URL"
  --variable "run_id=$RUN_ID"
)

echo "==> running Hurl setup" >&2
hurl_run --test \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  e2e/hurl/tag-generator/00-setup.hurl

echo "==> running Hurl suite (serial; SBERT warm-up in 03 precedes mq-hub round-trip in 04)" >&2
# --jobs 1 keeps ordering deterministic: 03 warms the SBERT model so
# that 04's round trip fits inside timeoutMs=15000 on a cold CI runner.
hurl_run --test \
  --jobs 1 \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  e2e/hurl/tag-generator/0[1-9]-*.hurl

echo "==> suite passed. reports: $REPORT_DIR" >&2
