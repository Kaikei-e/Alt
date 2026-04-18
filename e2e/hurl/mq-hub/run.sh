#!/usr/bin/env bash
# e2e/hurl/mq-hub/run.sh
#
# Brings up the mq-hub slice of the alt-staging stack, generates the
# oversize-batch fixture on the fly, runs the Hurl suite inside the
# alt-staging network (Hurl's Docker image joins the network so the
# `mq-hub` service DNS resolves), and tears down.
#
# The staging network is `internal: true`, which silently ignores host
# port publishes. Running Hurl inside the network is the only portable
# way to reach the SUT, and matches the CI pattern documented in
# compose/compose.staging.yaml.
#
# Environment overrides:
#   IMAGE_TAG    — tag of ghcr.io/${GHCR_OWNER}/alt-mq-hub image (default: main)
#   GHCR_OWNER   — GitHub container registry owner (default: kaikei-e)
#   BASE_URL     — mq-hub URL as seen from the Hurl container
#                  (default: http://mq-hub:9500 — service DNS on alt-staging)
#   HURL_IMAGE   — Hurl container image (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   RUN_ID       — unique run identifier for consumer-group isolation
#                  (default: $(date +%s))
#   KEEP_STACK=1 — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${IMAGE_TAG:=main}"
: "${GHCR_OWNER:=kaikei-e}"
: "${BASE_URL:=http://mq-hub:9500}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${RUN_ID:=$(date +%s)}"

REPORT_DIR="$ROOT/e2e/reports/mq-hub-$RUN_ID"
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

echo "==> generating oversize batch fixture (1001 events)" >&2
python3 e2e/fixtures/mq-hub/gen-batch-oversize.py \
  e2e/fixtures/mq-hub/batch-oversize.json

echo "==> bringing up redis-streams + mq-hub (IMAGE_TAG=$IMAGE_TAG, GHCR_OWNER=$GHCR_OWNER)" >&2
IMAGE_TAG="$IMAGE_TAG" GHCR_OWNER="$GHCR_OWNER" \
  docker compose -f compose/compose.staging.yaml -p alt-staging \
  up -d --wait redis-streams mq-hub

# Run Hurl inside the alt-staging network. Mount repo at the same path so
# `file,e2e/fixtures/...;` in the Hurl files resolves correctly.
hurl_run() {
  docker run --rm \
    --network alt-staging \
    -v "$ROOT:$ROOT" \
    -w "$ROOT" \
    "$HURL_IMAGE" \
    "$@"
}

echo "==> running Hurl setup (serial)" >&2
hurl_run --test \
  --file-root "$ROOT" \
  --variable "base_url=$BASE_URL" \
  --variable "run_id=$RUN_ID" \
  e2e/hurl/mq-hub/00-setup.hurl

echo "==> running Hurl suite (serial; state-modifying tests share streams)" >&2
hurl_run --test \
  --file-root "$ROOT" \
  --variable "base_url=$BASE_URL" \
  --variable "run_id=$RUN_ID" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  e2e/hurl/mq-hub/0[1-9]-*.hurl \
  e2e/hurl/mq-hub/1[0-9]-*.hurl

echo "==> suite passed. reports: $REPORT_DIR" >&2
