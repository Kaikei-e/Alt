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
#   RUN_ID          — unique run identifier for report directory naming
#                     (default: $(date +%s))
#   KEEP_STACK=1    — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${BASE_URL:=http://acolyte-orchestrator:8090}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${IMAGE_TAG:=main}"
: "${GHCR_OWNER:=kaikei-e}"
: "${RUN_ID:=$(date +%s)}"

export IMAGE_TAG GHCR_OWNER

# Master key for the Meilisearch seed step in scenario 09 (gatherer
# needs an indexed corpus to return non-empty hits). Anchored on the
# same fixture file the compose `secrets:` block mounts so changing
# one rotates both.
MEILI_MASTER_KEY="$(tr -d '\n' < "$ROOT/e2e/fixtures/staging-secrets/meili_master_key.txt")"

REPORT_DIR="$ROOT/e2e/reports/acolyte-orchestrator-$RUN_ID"
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

echo "==> bringing up acolyte-orchestrator staging slice" >&2
# --build is required because acolyte-db-migrator, acolyte-orchestrator,
# and news-creator-ollama-stub are local build contexts (no GHCR image).
# --wait blocks on healthcheck convergence; the migrator's restart=no +
# orchestrator's service_completed_successfully gate guarantees Atlas
# migration runs before the orchestrator boots.
#
# news-creator-ollama-stub is the destination of acolyte's
# OllamaGateway calls (/api/generate, /api/chat). search-indexer +
# meilisearch + stub-backend back the gatherer node's hybrid search.
# All four containers are profile-tagged with `acolyte-orchestrator`
# so a single --profile flag is enough.
docker compose -f compose/compose.staging.yaml -p alt-staging \
  --profile acolyte-orchestrator \
  up -d --wait --build \
  acolyte-db acolyte-db-migrator acolyte-orchestrator \
  news-creator-ollama-stub \
  meilisearch stub-backend search-indexer

# Run Hurl inside the alt-staging network. Mount the repo at the same
# absolute path so any `file,e2e/fixtures/...;` body resolves via
# --file-root "$ROOT".
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
  --variable "meili_master_key=$MEILI_MASTER_KEY" \
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

echo "==> running Hurl suite (serial; report→version→run FK chain requires ordering)" >&2
# --jobs 1 forces a single worker. acolyte-db has FK chains
# (report_versions → reports, report_runs → reports, change_items →
# report_versions) and several scenarios depend on captures from
# earlier scenarios (e.g. 04-07 use {{report_id_basic}} from 02).
# Hurl 7.1 --test defaults to parallel, which would shuffle this order.
hurl_run --test \
  --jobs 1 \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  "${suite_files[@]}"

echo "==> suite passed. reports: $REPORT_DIR" >&2
