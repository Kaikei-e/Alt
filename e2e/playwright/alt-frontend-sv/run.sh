#!/usr/bin/env bash
# RECLAIM_NETWORK_POOL: skip (this suite creates no compose network — it runs
# on --network host against a webServer and mock listeners inside the test
# process, so there is no address pool for it to exhaust or reclaim)
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
#   PLAYWRIGHT_IMAGE  — container image. Defaults to the tag matching the
#                       @playwright/test version in alt-frontend-sv/package.json,
#                       read at runtime rather than pinned (see below).
#   BUN_VERSION       — bun toolchain (default: 1.2.14; matches alt-frontend-sv.yml)
#   SHARD             — Playwright --shard value (default: 1/1; matrix sets 1/3|2/3|3/3)
#   PROJECTS          — space-separated --project flags (default: auth + desktop-chromium + mobile-chrome)
#   RUN_ID            — unique run identifier for report dir (default: $(date +%s))
#   REPORT_DIR        — override report output path (default: e2e/reports/alt-frontend-sv-<RUN_ID>)
#   KEEP_STACK=1      — no-op; kept for parity with Hurl run.sh ergonomics
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

# The container image tag MUST match the @playwright/test version the suite
# installs: the image ships browser binaries at a revision the driver pins, and
# a driver that finds the wrong revision either refuses to start ("Executable
# doesn't exist") or downloads ~400MB mid-run on a network the container may
# not have.
#
# This was pinned by hand and drifted: the workflow moved to v1.62.1-jammy
# while this file — the one alt-deploy actually invokes at release time — sat
# on v1.59.1-jammy. Reading the version out of package.json means the two
# cannot disagree again, and a bump in one place is a bump everywhere.
PLAYWRIGHT_VERSION="$(
  python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["devDependencies"]["@playwright/test"].lstrip("^~"))' \
    "$ROOT/alt-frontend-sv/package.json"
)"
: "${PLAYWRIGHT_IMAGE:=mcr.microsoft.com/playwright:v${PLAYWRIGHT_VERSION}-jammy}"
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
