#!/usr/bin/env bash
# e2e/hurl/recap-worker/run.sh
#
# Brings up the recap-worker slice of the alt-staging stack
# (recap-db + recap-db-migrator + recap-pipeline-stub + recap-worker),
# runs the Hurl suite inside the alt-staging network (the Hurl Docker
# image joins the network so `recap-worker` resolves via service DNS),
# and tears down.
#
# The alt-staging network is `internal: true`, which silently ignores
# host port publishes. Running Hurl inside the network is the only
# portable way to reach the SUT, matching search-indexer / mq-hub /
# knowledge-sovereign / tag-generator.
#
# The recap pipeline calls four upstream services (recap-subworker,
# news-creator, alt-backend, tag-generator). recap-pipeline-stub is
# multi-aliased on the alt-staging network as all four hostnames so
# every upstream URL the worker holds resolves to the stub.
#
# Environment overrides:
#   BASE_URL    — recap-worker URL as seen from the Hurl container
#                 (default: http://recap-worker:9005)
#   HURL_IMAGE  — Hurl container image
#                 (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   IMAGE_TAG   — tag of ghcr.io/<owner>/alt-recap-worker +
#                 alt-recap-pipeline-stub (default: main)
#   GHCR_OWNER  — GHCR namespace (default: kaikei-e)
#   RUN_ID               — unique run identifier (default: $(date +%s))
#   STAGING_PROJECT_NAME — compose project + network name (default: alt-staging).
#                          CI sets alt-staging-recap-worker so parallel matrix
#                          jobs on the shared Docker daemon don't collide.
#   KEEP_STACK=1         — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${IMAGE_TAG:=main}"
: "${GHCR_OWNER:=kaikei-e}"
: "${BASE_URL:=http://recap-worker:9005}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${RUN_ID:=$(date +%s)}"
: "${STAGING_PROJECT_NAME:=alt-staging}"

# Per-service image tag: see search-indexer/run.sh for rationale.
# recap-pipeline-stub is a test fixture built in-tree at the same
# rhythm as recap-worker, so it piggybacks on the same IMAGE_TAG.
: "${RECAP_WORKER_IMAGE_TAG:=$IMAGE_TAG}"
: "${RECAP_PIPELINE_STUB_IMAGE_TAG:=$IMAGE_TAG}"
export IMAGE_TAG GHCR_OWNER STAGING_PROJECT_NAME RECAP_WORKER_IMAGE_TAG RECAP_PIPELINE_STUB_IMAGE_TAG

# shellcheck source=../_lib/render-slice.sh
source "$ROOT/e2e/hurl/_lib/render-slice.sh"
render_slice recap-worker

REPORT_DIR="$ROOT/e2e/reports/recap-worker-$RUN_ID"
mkdir -p "$REPORT_DIR"

cleanup() {
  if [[ "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> tearing down $STAGING_PROJECT_NAME stack" >&2
    # --remove-orphans + -v removes the anonymous postgres volume
    # attached to recap-db. Without -v the next run resurrects the old
    # schema + rows and scenario 03 sees pre-existing recap_outputs.
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
      down -v --remove-orphans >/dev/null 2>&1 || true
  else
    echo "==> KEEP_STACK=1 — leaving $STAGING_PROJECT_NAME stack up" >&2
  fi
  rm -rf "$SLICE_DIR"
}
trap cleanup EXIT

echo "==> bringing up recap-worker slice ($STAGING_PROJECT_NAME)" >&2
docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
  up -d --wait recap-db recap-pipeline-stub recap-worker

# recap-db-migrator is a one-shot job (restart: "no") that exits when
# done; --wait would poll forever, so let `depends_on:
# service_completed_successfully` on recap-worker gate it instead.

# Run Hurl inside the staging network so in-network DNS resolves
# the recap-worker service. Mount the repo at the same absolute path
# so any `file,e2e/fixtures/...;` body resolves via --file-root "$ROOT".
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
  --variable "run_id=$RUN_ID"
)

echo "==> running Hurl setup" >&2
hurl_run --test \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  e2e/hurl/recap-worker/00-setup.hurl

echo "==> running Hurl suite (--jobs 1; scenarios 05/06 mutate recap_outputs)" >&2
# --jobs 1 keeps ordering deterministic: 03's empty-DB assertion must
# observe an empty recap_outputs table before 05/06 trigger their
# pipelines. Per ADR-000766 this matches the dispatch convention.
hurl_run --test \
  --jobs 1 \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  e2e/hurl/recap-worker/0[1-9]-*.hurl

echo "==> suite passed. reports: $REPORT_DIR" >&2
