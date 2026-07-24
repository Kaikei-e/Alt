#!/usr/bin/env bash
# e2e/hurl/acolyte-orchestrator/run.sh
#
# Brings up the acolyte-orchestrator slice of the alt-staging stack
# (acolyte-db Postgres + Atlas migrator + orchestrator), runs the Hurl
# suite inside the alt-staging network (Hurl's Docker image joins the
# network so the `acolyte-orchestrator` service DNS resolves), and tears
# down. The migrator runs `atlas migrate apply` against the freshly
# booted Postgres before the orchestrator is allowed to start.
#
# The staging network is `internal: true`, which silently ignores host
# port publishes. Running Hurl inside the network is the only portable
# way to reach the SUT, matching the search-indexer / mq-hub /
# knowledge-sovereign / news-creator pattern.
#
# Environment overrides:
#   BASE_URL        — orchestrator URL as seen from the Hurl container
#                     (default: http://acolyte-orchestrator:8090)
#   HURL_IMAGE      — Hurl container image
#                     (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   DIAG_CURL_IMAGE      — small curl image used by dump_diagnostics to
#                          probe search-indexer/meilisearch in-network
#                          (default: curlimages/curl:8.7.1)
#   RUN_ID               — unique run identifier for report directory naming
#                          (default: $(date +%s))
#   STAGING_PROJECT_NAME — compose project + network name (default: alt-staging).
#                          CI sets alt-staging-acolyte-orchestrator so parallel
#                          matrix jobs on the shared Docker daemon don't collide.
#   KEEP_STACK=1         — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${BASE_URL:=http://acolyte-orchestrator:8090}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${DIAG_CURL_IMAGE:=curlimages/curl:8.7.1}"
: "${IMAGE_TAG:=main}"
: "${GHCR_OWNER:=kaikei-e}"
: "${RUN_ID:=$(date +%s)}"
: "${STAGING_PROJECT_NAME:=alt-staging}"

# acolyte-orchestrator itself is built locally (alt-staging/
# acolyte-orchestrator:local) so only the GHCR-pulled dependencies —
# search-indexer, news-creator, stub-backend — need per-service image
# tags. They default to `main` in compose.staging.yaml so this suite
# does not have to override any of them: unrelated dependency builds
# that did not get rebuilt at the dispatch SHA still resolve.
export IMAGE_TAG GHCR_OWNER STAGING_PROJECT_NAME

# shellcheck source=../_lib/render-slice.sh
source "$ROOT/e2e/hurl/_lib/render-slice.sh"
render_slice acolyte-orchestrator

# Pre-cleanup: reclaim Docker's pre-defined address pool from networks
# left by cancelled prior runs. Safe-by-default: docker network prune
# refuses to touch networks an active container is attached to.
# shellcheck source=../_lib/reclaim-network-pool.sh
source "$ROOT/e2e/hurl/_lib/reclaim-network-pool.sh"
reclaim_network_pool

# shellcheck source=../_lib/compose-up-with-retry.sh
source "$ROOT/e2e/hurl/_lib/compose-up-with-retry.sh"

# Master key for the Meilisearch seed step in scenario 09 (gatherer
# needs an indexed corpus to return non-empty hits). Anchored on the
# same fixture file the compose `secrets:` block mounts so changing
# one rotates both.
MEILI_MASTER_KEY="$(tr -d '\n' < "$ROOT/e2e/fixtures/staging-secrets/meili_master_key.txt")"

REPORT_DIR="$ROOT/e2e/reports/acolyte-orchestrator-$RUN_ID"
mkdir -p "$REPORT_DIR"

cleanup() {
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

echo "==> bringing up acolyte-orchestrator slice ($STAGING_PROJECT_NAME)" >&2
# --build is required because acolyte-db-migrator, acolyte-orchestrator,
# and news-creator-ollama-stub are local build contexts (no GHCR image).
# --wait blocks on healthcheck convergence; the migrator's restart=no +
# orchestrator's service_completed_successfully gate guarantees Atlas
# migration runs before the orchestrator boots.
#
# news-creator-ollama-stub is the destination of acolyte's
# OllamaGateway calls (/api/generate, /api/chat). search-indexer +
# meilisearch + stub-backend back the gatherer node's hybrid search.
compose_up_with_retry --build \
  acolyte-db acolyte-db-migrator acolyte-orchestrator \
  news-creator-ollama-stub \
  meilisearch stub-backend search-indexer

# Run Hurl inside the staging network. Mount the repo at the same
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
  --variable "run_id=$RUN_ID"
  --variable "title_prefix=hurl-$RUN_ID"
)

echo "==> running Hurl setup (readiness gate)" >&2
hurl_run --test \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  e2e/hurl/acolyte-orchestrator/00-setup.hurl

echo "==> seeding meilisearch articles index for gatherer node" >&2
# Reuses the canonical search-indexer seed fixture. The Acolyte run
# scenarios depend on a non-empty index so the gatherer node returns
# evidence; without it the curator/writer nodes get empty input and
# the pipeline still completes but with degraded content shape.
hurl_run --test \
  --file-root "$ROOT" \
  --secret "meili_master_key=$MEILI_MASTER_KEY" \
  e2e/hurl/search-indexer/00-seed-meilisearch.hurl

# Collect suite files via nullglob so future increments can land
# 1[0-9]-*.hurl and 2[0-9]-*.hurl without script edits, and so the
# current set runs cleanly even when those slots are still empty.
shopt -s nullglob
suite_files=(
  e2e/hurl/acolyte-orchestrator/0[1-9]-*.hurl
  e2e/hurl/acolyte-orchestrator/1[0-9]-*.hurl
  e2e/hurl/acolyte-orchestrator/2[0-9]-*.hurl
)
shopt -u nullglob

dump_diagnostics() {
  # Best-effort: containers may already be half-gone by the time a
  # failure is diagnosed, and none of this may mask the suite's own
  # exit code. Written into $REPORT_DIR so CI's upload-artifact step
  # (which only globs e2e/reports/acolyte-orchestrator-*) picks these
  # up alongside the junit/html reports — the CI job itself never
  # retains container logs otherwise.
  echo "==> dumping failure diagnostics to $REPORT_DIR" >&2
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
    logs --tail 300 acolyte-orchestrator \
    > "$REPORT_DIR/diagnostics-orchestrator.log" 2>&1 || true
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
    logs --tail 300 search-indexer \
    > "$REPORT_DIR/diagnostics-search-indexer.log" 2>&1 || true
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
    exec -T acolyte-db psql -U acolyte -d acolyte -c \
    "SELECT run_id, run_status, failure_code, failure_message, started_at, finished_at FROM report_runs ORDER BY started_at DESC NULLS LAST LIMIT 5;" \
    > "$REPORT_DIR/diagnostics-report-runs.txt" 2>&1 || true

  # Which image (Repository/Tag/Image ID) actually ran — pins down
  # "which build was live" before chasing behavior, e.g. a stale GHCR
  # `main` tag still serving pre-fix search-indexer content-cropping.
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
    images \
    > "$REPORT_DIR/diagnostics-images.txt" 2>&1 || true
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
    ps --format json \
    >> "$REPORT_DIR/diagnostics-images.txt" 2>&1 || true

  # Bypass the orchestrator and gatherer node entirely and hit
  # search-indexer's REST API straight from inside the staging network
  # (same small-image-in-network shape as hurl_run). Confirms whether
  # hits/content are already empty at the search-indexer boundary,
  # independent of anything acolyte does with the response afterward.
  docker run --rm --network "$STAGING_PROJECT_NAME" "$DIAG_CURL_IMAGE" \
    -s "http://search-indexer:9300/v1/search?q=GPU&limit=2" \
    > "$REPORT_DIR/diagnostics-probe-search-indexer.json" 2>&1 || true

  # Same query straight against Meilisearch with attributesToCrop, to
  # tell apart "Meilisearch has no content for this doc" from
  # "search-indexer drops content on the way out". MEILI_MASTER_KEY is
  # only ever placed in the Authorization header here — `curl -s` on
  # success prints just the response body, and on failure only curl's
  # own connection-error text, so the key value never lands in a file.
  docker run --rm --network "$STAGING_PROJECT_NAME" "$DIAG_CURL_IMAGE" \
    -s -X POST "http://meilisearch:7700/indexes/articles/search" \
    -H "Authorization: Bearer $MEILI_MASTER_KEY" \
    -H "Content-Type: application/json" \
    -d '{"q":"GPU","limit":2,"attributesToRetrieve":["id","title"],"attributesToCrop":["content"],"cropLength":120}' \
    > "$REPORT_DIR/diagnostics-probe-meilisearch.json" 2>&1 || true

  # /version is gated behind the master key too (same as /indexes/*),
  # unlike /health — without Authorization this just returns a
  # missing_authorization_header error instead of a version string.
  docker run --rm --network "$STAGING_PROJECT_NAME" "$DIAG_CURL_IMAGE" \
    -s "http://meilisearch:7700/version" \
    -H "Authorization: Bearer $MEILI_MASTER_KEY" \
    > "$REPORT_DIR/diagnostics-meili-version.txt" 2>&1 || true
}

echo "==> running Hurl suite (serial; report→version→run FK chain requires ordering)" >&2
# --jobs 1 forces a single worker. acolyte-db has FK chains
# (report_versions → reports, report_runs → reports, change_items →
# report_versions) and several scenarios depend on captures from
# earlier scenarios (e.g. 04-07 use {{report_id_basic}} from 02).
# Hurl 7.1 --test defaults to parallel, which would shuffle this order.
if hurl_run --test \
  --jobs 1 \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  "${suite_files[@]}"; then
  echo "==> suite passed. reports: $REPORT_DIR" >&2
else
  suite_status=$?
  echo "==> Hurl suite failed (exit $suite_status)" >&2
  dump_diagnostics
  exit "$suite_status"
fi
