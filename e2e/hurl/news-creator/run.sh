#!/usr/bin/env bash
# e2e/hurl/news-creator/run.sh
#
# Brings up the news-creator slice of the alt-staging stack
# (news-creator-ollama-stub + news-creator), runs the Hurl suite inside
# the alt-staging network (Hurl's Docker image joins the network so the
# `news-creator` service DNS resolves), and tears down.
#
# The alt-staging network is `internal: true`, which silently ignores
# host port publishes. Running Hurl inside the network is the only
# portable way to reach the SUT, matching search-indexer / mq-hub /
# knowledge-sovereign / tag-generator.
#
# Phase 1 scope: readiness + /health schema + /queue/status schema.
# Real Ollama is replaced by a Python/FastAPI stub that returns a fixed
# `/api/tags` response. WARMUP_ENABLED=false in compose.staging.yaml
# keeps the FastAPI lifespan from calling /api/generate against the stub.
#
# Environment overrides:
#   BASE_URL        — news-creator URL as seen from the Hurl container
#                     (default: http://news-creator:11434)
#   HURL_IMAGE      — Hurl container image
#                     (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   IMAGE_TAG       — tag of ghcr.io/<owner>/alt-news-creator
#                     (default: main)
#   GHCR_OWNER      — GHCR namespace (default: kaikei-e)
#   RUN_ID          — unique run identifier for report directory naming
#                     (default: $(date +%s))
#   KEEP_STACK=1    — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${IMAGE_TAG:=main}"
: "${GHCR_OWNER:=kaikei-e}"
: "${BASE_URL:=http://news-creator:11434}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${RUN_ID:=$(date +%s)}"

export IMAGE_TAG GHCR_OWNER

REPORT_DIR="$ROOT/e2e/reports/news-creator-$RUN_ID"
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

echo "==> bringing up news-creator staging slice" >&2
docker compose -f compose/compose.staging.yaml -p alt-staging \
  --profile news-creator \
  up -d --wait --build news-creator-ollama-stub news-creator

# Run Hurl inside the alt-staging network so the `news-creator` service
# DNS resolves. Mount the repo at the same absolute path so any
# `file,e2e/fixtures/...;` body resolves via --file-root "$ROOT".
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
)

echo "==> running Hurl setup" >&2
hurl_run --test \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  e2e/hurl/news-creator/00-setup.hurl

echo "==> running Hurl suite (serial; queue + semaphore state is shared)" >&2
# --jobs 1 keeps ordering deterministic and prevents Phase-4 queue
# saturation scenarios (added in a later increment) from racing the
# health/queue scenarios introduced here in Phase 1.
hurl_run --test \
  --jobs 1 \
  --file-root "$ROOT" \
  "${common_vars[@]}" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  e2e/hurl/news-creator/0[1-9]-*.hurl \
  e2e/hurl/news-creator/1[0-9]-*.hurl

echo "==> suite passed. reports: $REPORT_DIR" >&2
