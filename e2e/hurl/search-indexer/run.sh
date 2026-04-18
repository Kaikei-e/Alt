#!/usr/bin/env bash
# e2e/hurl/search-indexer/run.sh
#
# Brings up the search-indexer slice of the alt-staging stack
# (meilisearch + stub-backend + search-indexer), seeds the articles
# index with the fixture corpus, runs the Hurl suite inside the
# alt-staging network (Hurl's Docker image joins the network so the
# `meilisearch` and `search-indexer` service DNS resolve), and tears
# down.
#
# The staging network is `internal: true`, which silently ignores host
# port publishes. Running Hurl inside the network is the only portable
# way to reach the SUT, and matches the mq-hub / knowledge-sovereign
# pattern.
#
# Environment overrides:
#   IMAGE_TAG        — tag of ghcr.io/${GHCR_OWNER}/alt-search-indexer image
#                      (default: main)
#   GHCR_OWNER       — GitHub container registry owner (default: kaikei-e)
#   HURL_IMAGE       — Hurl container image
#                      (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   RUN_ID           — unique run identifier (default: $(date +%s))
#   KEEP_STACK=1     — do not tear the stack down on exit (for debugging).
#                      Caution: on persistent runners this leaves
#                      container_name conflicts for the next run.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${IMAGE_TAG:=main}"
: "${GHCR_OWNER:=kaikei-e}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${RUN_ID:=$(date +%s)}"

# Master key is a committed staging-only test fixture. Anchor through
# the same file the compose `secrets:` block mounts, so changing one
# rotates both.
MEILI_MASTER_KEY="$(tr -d '\n' < "$ROOT/e2e/fixtures/staging-secrets/meili_master_key.txt")"

REPORT_DIR="$ROOT/e2e/reports/search-indexer-$RUN_ID"
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

echo "==> bringing up search-indexer slice (IMAGE_TAG=$IMAGE_TAG, GHCR_OWNER=$GHCR_OWNER)" >&2
IMAGE_TAG="$IMAGE_TAG" GHCR_OWNER="$GHCR_OWNER" \
  docker compose -f compose/compose.staging.yaml -p alt-staging \
  --profile search-indexer \
  up -d --wait meilisearch stub-backend search-indexer

# Run Hurl inside the alt-staging network. Mount repo at the same path
# so `file,e2e/fixtures/...;` body refs resolve via --file-root "$ROOT".
hurl_run() {
  docker run --rm \
    --network alt-staging \
    -v "$ROOT:$ROOT" \
    -w "$ROOT" \
    "$HURL_IMAGE" \
    "$@"
}

echo "==> seeding meilisearch articles index (serial)" >&2
hurl_run --test \
  --file-root "$ROOT" \
  --variable "meili_master_key=$MEILI_MASTER_KEY" \
  e2e/hurl/search-indexer/00-seed-meilisearch.hurl

echo "==> running Hurl suite (parallel — REST search is stateless)" >&2
hurl_run --test \
  --jobs 4 \
  --retry 5 \
  --retry-interval 500 \
  --file-root "$ROOT" \
  --variable "meili_master_key=$MEILI_MASTER_KEY" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  e2e/hurl/search-indexer/0[1-9]-*.hurl

echo "==> suite passed. reports: $REPORT_DIR" >&2
