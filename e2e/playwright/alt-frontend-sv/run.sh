#!/usr/bin/env bash
# e2e/playwright/alt-frontend-sv/run.sh
#
# Dispatches the alt-frontend-sv Playwright E2E suite inside a Playwright
# container. Parallels the Hurl `e2e/hurl/<svc>/run.sh` convention
# (ADR-000766) so alt-deploy's release-deploy.yaml can matrix over
# `bash e2e/<framework>/<svc>/run.sh` with a single two-directory probe.
#
# The suite is self-contained: `playwright.config.ts` webServer boots
# `bun run build && node build` on 4174, and `tests/e2e/global-setup.ts`
# spins up mock Kratos (4001), AuthHub (4002), and Backend (4003) inside
# the test process. No compose.staging.yaml profile is required.
#
# Environment overrides:
#   PLAYWRIGHT_IMAGE  — container image (default: mcr.microsoft.com/playwright:v1.59.1-jammy)
#   BUN_VERSION       — bun toolchain (default: 1.2.14; matches alt-frontend-sv.yml)
#   SHARD             — Playwright --shard value (default: 1/1; matrix sets 1/3|2/3|3/3)
#   PROJECTS          — space-separated --project flags (default: auth + desktop-chromium + mobile-chrome)
#   RUN_ID            — unique run identifier for report dir (default: $(date +%s))
#   REPORT_DIR        — override report output path (default: e2e/reports/alt-frontend-sv-<RUN_ID>)
#   KEEP_STACK=1      — no-op; kept for parity with Hurl run.sh ergonomics
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${PLAYWRIGHT_IMAGE:=mcr.microsoft.com/playwright:v1.59.1-jammy}"
: "${BUN_VERSION:=1.2.14}"
: "${SHARD:=1/1}"
: "${PROJECTS:=--project=auth --project=desktop-chromium --project=mobile-chrome}"
: "${RUN_ID:=$(date +%s)}"
: "${REPORT_DIR:=$ROOT/e2e/reports/alt-frontend-sv-$RUN_ID}"

mkdir -p "$REPORT_DIR"

# Shard value must be sharable with the Playwright CLI's regex guard.
# Matrix jobs pass 1/3|2/3|3/3; local runs default to 1/1.
if ! [[ "$SHARD" =~ ^[0-9]+/[0-9]+$ ]]; then
  echo "SHARD must match N/M (got: $SHARD)" >&2
  exit 2
fi

# The Playwright container needs: unzip (bun installer prereq), bun
# itself, and network host so `global-setup.ts` can bind 127.0.0.1:4001-4003
# while `webServer` binds 4174. The in-container sequence mirrors
# .github/workflows/alt-frontend-sv.yml step-for-step so CI green and
# local run.sh green cannot diverge.
echo "==> running Playwright ($SHARD) in $PLAYWRIGHT_IMAGE" >&2
docker run --rm \
  --network host \
  -v "$ROOT:$ROOT" \
  -w "$ROOT/alt-frontend-sv" \
  -e CI=true \
  -e FORCE_COLOR=1 \
  -e PLAYWRIGHT_SKIP_GIT_INFO=1 \
  -e BUN_INSTALL=/root/.bun \
  -e BUN_VERSION="$BUN_VERSION" \
  -e PROJECTS="$PROJECTS" \
  -e SHARD="$SHARD" \
  -e REPORT_DIR="$REPORT_DIR" \
  "$PLAYWRIGHT_IMAGE" \
  bash -euxo pipefail -c '
    for attempt in 1 2 3; do
      apt-get clean
      rm -rf /var/lib/apt/lists/*
      if apt-get update && apt-get install -y --no-install-recommends unzip ca-certificates curl; then
        break
      fi
      if [ "$attempt" = "3" ]; then
        echo "apt install failed after 3 attempts" >&2
        exit 1
      fi
      sleep $((attempt * 15))
    done

    curl -fsSL https://bun.sh/install | bash -s -- "bun-v${BUN_VERSION}"
    export PATH="${BUN_INSTALL}/bin:${PATH}"
    bun --version

    bun install --frozen-lockfile

    bunx playwright test ${PROJECTS} --shard="${SHARD}"
  '

echo "==> suite passed. reports: $REPORT_DIR" >&2
