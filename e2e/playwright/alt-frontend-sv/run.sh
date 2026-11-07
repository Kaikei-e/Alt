#!/usr/bin/env bash
# RECLAIM_NETWORK_POOL: skip (this suite creates no compose network — it runs
# on --network host against the pre-built app container and mock listeners
# inside the test process, so there is no address pool for it to exhaust or
# reclaim)
# e2e/playwright/alt-frontend-sv/run.sh
#
# Dispatches the alt-frontend-sv Playwright E2E suite against the pre-built
# `alt-alt-frontend-sv` image.
#
# `alt-frontend-sv/src/env.ts` reads all backend URLs and ORIGIN from runtime
# environment variables (`$app/env/private`), so the production image serves the
# E2E suite when started on `--network host` with the mock endpoints on
# 127.0.0.1:4001-4003 (started by `tests/e2e/global-setup.ts`). Passing
# `PW_NO_WEBSERVER=1` disables `playwright.config.ts`'s `bun run build && node
# build` webServer, and the Playwright container installs only the three E2E
# runner packages (`@playwright/test`, `axe-playwright`, `jsonwebtoken`) into
# `/work` rather than installing Bun and the full frontend dependency tree.
#
# Environment overrides:
#   IMAGE_TAG                 — Docker tag for the service image (default: ci)
#   GHCR_OWNER                — GHCR namespace (default: kaikei-e)
#   ALT_FRONTEND_SV_IMAGE_TAG — override tag for alt-alt-frontend-sv (default: $IMAGE_TAG)
#   ALT_FRONTEND_SV_IMAGE     — full image ref override
#   ALT_E2E_PORT              — port for the frontend server (default: 4174)
#   PLAYWRIGHT_IMAGE          — browser runner image (defaults to version in package.json)
#   SHARD                     — Playwright --shard value (default: 1/1)
#   PROJECTS                  — space-separated --project flags
#   RUN_ID                    — unique run identifier for report dir
#   REPORT_DIR                — override report output path
#   KEEP_STACK=1              — leave the app container up on exit for inspection
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

# The container image tag MUST match the @playwright/test version the suite
# installs: the image ships browser binaries at a revision the driver pins.
PLAYWRIGHT_VERSION="$(
  python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["devDependencies"]["@playwright/test"].lstrip("^~"))' \
    "$ROOT/alt-frontend-sv/package.json"
)"
: "${IMAGE_TAG:=ci}"
: "${GHCR_OWNER:=kaikei-e}"
: "${ALT_FRONTEND_SV_IMAGE_TAG:=$IMAGE_TAG}"
: "${ALT_FRONTEND_SV_IMAGE:=ghcr.io/${GHCR_OWNER}/alt-alt-frontend-sv:${ALT_FRONTEND_SV_IMAGE_TAG}}"
: "${ALT_E2E_PORT:=4174}"
: "${PLAYWRIGHT_IMAGE:=mcr.microsoft.com/playwright:v${PLAYWRIGHT_VERSION}-jammy}"
: "${SHARD:=1/1}"
: "${PROJECTS:=--project=auth --project=desktop-chromium --project=mobile-chrome}"
: "${RUN_ID:=$(date +%s)}"
: "${REPORT_DIR:=$ROOT/e2e/reports/alt-frontend-sv-$RUN_ID}"
: "${STAGING_PROJECT_NAME:=alt-staging-alt-frontend-sv}"

mkdir -p "$REPORT_DIR"

if ! [[ "$SHARD" =~ ^[0-9]+/[0-9]+$ ]]; then
  echo "SHARD must match N/M (got: $SHARD)" >&2
  exit 2
fi

E2E_ORIGIN="http://127.0.0.1:${ALT_E2E_PORT}"
SAFE_RUN_ID="${RUN_ID//[^a-zA-Z0-9_.-]/-}"
APP_CONTAINER="alt-e2e-frontend-sv-${SAFE_RUN_ID}-$$"

cleanup() {
  local rc=$?
  if [[ $rc -ne 0 ]]; then
    echo "::group::alt-frontend-sv container logs" >&2
    docker logs --tail 200 "$APP_CONTAINER" >&2 || true
    echo "::endgroup::" >&2
  fi
  if [[ "${KEEP_STACK:-0}" != "1" ]]; then
    docker rm -f "$APP_CONTAINER" >/dev/null 2>&1 || true
  fi
  exit "$rc"
}
trap cleanup EXIT

# Remove any leftover container from a prior shard or cancelled run so
# 127.0.0.1:${ALT_E2E_PORT} is free before starting the new container.
stale_containers="$(docker ps -aq --filter "label=alt.e2e.suite=alt-frontend-sv" 2>/dev/null || true)"
if [[ -n "$stale_containers" ]]; then
  # shellcheck disable=SC2086
  docker rm -f $stale_containers >/dev/null 2>&1 || true
fi

echo "==> starting pre-built frontend ($ALT_FRONTEND_SV_IMAGE) on $E2E_ORIGIN" >&2
docker run -d \
  --name "$APP_CONTAINER" \
  --label "alt.e2e.suite=alt-frontend-sv" \
  --label "com.docker.compose.project=${STAGING_PROJECT_NAME}" \
  --network host \
  -e PORT="$ALT_E2E_PORT" \
  -e HOST=127.0.0.1 \
  -e ORIGIN="$E2E_ORIGIN" \
  -e NODE_ENV=production \
  -e E2E_TEST_MODE=true \
  -e KRATOS_INTERNAL_URL="http://127.0.0.1:4001" \
  -e KRATOS_PUBLIC_URL="http://127.0.0.1:4001" \
  -e AUTH_HUB_INTERNAL_URL="http://127.0.0.1:4002" \
  -e BACKEND_BASE_URL="http://127.0.0.1:4003" \
  -e BACKEND_REST_URL="http://127.0.0.1:4003" \
  -e BACKEND_CONNECT_URL="http://127.0.0.1:4003" \
  -e BFF_CONNECT_URL="http://127.0.0.1:4003" \
  -e RECAP_WORKER_BASE_URL="http://127.0.0.1:4003" \
  -e SOVEREIGN_ADMIN_AUTH=disabled \
  "$ALT_FRONTEND_SV_IMAGE" >/dev/null

for attempt in $(seq 1 60); do
  if curl -fsS "${E2E_ORIGIN}/health" >/dev/null 2>&1; then
    break
  fi
  if [[ "$attempt" -eq 60 ]]; then
    echo "alt-frontend-sv did not become healthy at ${E2E_ORIGIN}/health within 60s" >&2
    exit 1
  fi
  sleep 1
done

echo "==> running Playwright ($SHARD) in $PLAYWRIGHT_IMAGE" >&2
docker run --rm \
  --network host \
  -v "$ROOT:/work/repo" \
  -v "$REPORT_DIR:$REPORT_DIR" \
  -w /work/repo/alt-frontend-sv \
  -e CI=true \
  -e PW_NO_WEBSERVER=1 \
  -e ALT_E2E_PORT="$ALT_E2E_PORT" \
  -e FORCE_COLOR=1 \
  -e PLAYWRIGHT_SKIP_GIT_INFO=1 \
  -e PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
  -e PROJECTS="$PROJECTS" \
  -e SHARD="$SHARD" \
  -e REPORT_DIR="$REPORT_DIR" \
  "$PLAYWRIGHT_IMAGE" \
  bash -euxo pipefail -c '
    # Install only the external packages imported under tests/e2e/ and
    # playwright.config.ts into /work (an ancestor of /work/repo/alt-frontend-sv
    # on container overlayfs so Node ESM/CJS resolution finds /work/node_modules
    # without writing root-owned files into the bind-mounted checkout).
    node -e '\''
      const fs = require("node:fs");
      const pkg = JSON.parse(fs.readFileSync("/work/repo/alt-frontend-sv/package.json", "utf8"));
      const pick = (name) => pkg.devDependencies?.[name] ?? pkg.dependencies?.[name];
      fs.writeFileSync("/work/package.json", JSON.stringify({
        name: "alt-frontend-sv-e2e-runner",
        private: true,
        devDependencies: {
          "@playwright/test": pick("@playwright/test"),
          "axe-playwright": pick("axe-playwright"),
          "jsonwebtoken": pick("jsonwebtoken"),
        },
      }, null, 2));
    '\''

    for attempt in 1 2 3; do
      if npm --prefix /work install --no-audit --no-fund --no-save --prefer-offline --loglevel=error; then
        break
      fi
      if [ "$attempt" = "3" ]; then
        echo "npm install failed after 3 attempts" >&2
        exit 1
      fi
      sleep $((attempt * 5))
    done

    # shellcheck disable=SC2086
    /work/node_modules/.bin/playwright test ${PROJECTS} --shard="${SHARD}"
  '

echo "==> suite passed. reports: $REPORT_DIR" >&2
