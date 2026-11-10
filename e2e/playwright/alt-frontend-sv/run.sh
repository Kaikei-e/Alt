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
# `PW_NO_WEBSERVER=1` disables `playwright.config.ts`'s `webServer` command, and
# the Playwright container installs only the E2E runner dependencies into `/work`
# rather than installing Bun and the full frontend dependency tree.
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
#   PW_BLOB=1                 — emit a blob report into $REPORT_DIR/blob for merge-reports
#   KEEP_STACK=1              — leave the app container up on exit for inspection
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

# The container image tag MUST match the @playwright/test version the suite
# installs: the image ships browser binaries at a revision the driver pins.
PLAYWRIGHT_VERSION="$(
  python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["devDependencies"]["@playwright/test"])' \
    "$ROOT/e2e/playwright/package.json"
)"
: "${IMAGE_TAG:=ci}"
: "${GHCR_OWNER:=kaikei-e}"
: "${ALT_FRONTEND_SV_IMAGE_TAG:=$IMAGE_TAG}"
: "${ALT_FRONTEND_SV_IMAGE:=ghcr.io/${GHCR_OWNER}/alt-alt-frontend-sv:${ALT_FRONTEND_SV_IMAGE_TAG}}"
: "${ALT_E2E_PORT:=4174}"
: "${PLAYWRIGHT_IMAGE:=mcr.microsoft.com/playwright:v${PLAYWRIGHT_VERSION}-jammy}"
: "${NPM_CI_MAX_ATTEMPTS:=3}"
: "${NPM_CI_BACKOFF_SECS:=10}"
: "${SHARD:=1/1}"
: "${PROJECTS:=--project=auth --project=desktop-chromium --project=mobile-chrome}"
: "${RUN_ID:=$(date +%s)}"
: "${REPORT_DIR:=$ROOT/e2e/reports/alt-frontend-sv-$RUN_ID}"

mkdir -p "$REPORT_DIR"

if ! [[ "$SHARD" =~ ^[0-9]+/[0-9]+$ ]]; then
  echo "SHARD must match N/M (got: $SHARD)" >&2
  exit 2
fi

E2E_ORIGIN="http://127.0.0.1:${ALT_E2E_PORT}"
SAFE_RUN_ID="${RUN_ID//[^a-zA-Z0-9_.-]/-}"
: "${STAGING_PROJECT_NAME:=alt-staging-alt-frontend-sv-${SAFE_RUN_ID}}"
APP_CONTAINER="alt-e2e-frontend-sv-${SAFE_RUN_ID}-$$"

cleanup() {
  local rc=$?
  docker logs "$APP_CONTAINER" >"$REPORT_DIR/app-container.log" 2>&1 || true
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

# Remove any leftover container belonging to this project/run before checking
# whether the target port is free.
stale_containers="$(
  docker ps -aq \
    --filter "label=alt.e2e.suite=alt-frontend-sv" \
    --filter "label=com.docker.compose.project=${STAGING_PROJECT_NAME}" \
    2>/dev/null || true
)"
if [[ -n "$stale_containers" ]]; then
  # shellcheck disable=SC2086
  docker rm -f $stale_containers >/dev/null 2>&1 || true
fi

# Guard against running green or red against an unrelated process already bound
# to ${ALT_E2E_PORT} (the same invariant `reuseExistingServer: false` enforced
# when Playwright managed webServer).
if curl -fsS --max-time 2 "${E2E_ORIGIN}/health" >/dev/null 2>&1; then
  echo "refusing to start: ${E2E_ORIGIN}/health is already answering before container launch (set ALT_E2E_PORT to choose an unused port)" >&2
  exit 1
fi

echo "==> starting pre-built frontend ($ALT_FRONTEND_SV_IMAGE) on $E2E_ORIGIN" >&2
# The compose project label lets `e2e/_lib/teardown-project.sh` collect this
# container after a `KEEP_STACK=1` run or an external cancellation.
# PROTOCOL_HEADER stands in for the production TLS proxy so adapter-node derives http:// on loopback.
docker run -d \
  --name "$APP_CONTAINER" \
  --label "alt.e2e.suite=alt-frontend-sv" \
  --label "com.docker.compose.project=${STAGING_PROJECT_NAME}" \
  --network host \
  -e PORT="$ALT_E2E_PORT" \
  -e HOST=127.0.0.1 \
  -e ORIGIN="$E2E_ORIGIN" \
  -e PROTOCOL_HEADER=x-forwarded-proto \
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
  if [[ "$(docker inspect -f '{{.State.Running}}' "$APP_CONTAINER" 2>/dev/null || echo false)" != "true" ]]; then
    echo "alt-frontend-sv container ($APP_CONTAINER) exited before becoming healthy" >&2
    exit 1
  fi
  if curl -fsS --max-time 2 "${E2E_ORIGIN}/health" >/dev/null 2>&1; then
    break
  fi
  if [[ "$attempt" -eq 60 ]]; then
    echo "alt-frontend-sv did not become healthy at ${E2E_ORIGIN}/health within 60s" >&2
    exit 1
  fi
  sleep 1
done

echo "==> running Playwright ($SHARD) in $PLAYWRIGHT_IMAGE" >&2
# Mask any host `alt-frontend-sv/node_modules` with a tmpfs so Node never loads
# a second copy of `@playwright/test` from the checkout ahead of `/work/node_modules`.
docker run --rm \
  --network host \
  -v "$ROOT:/work/repo" \
  -v "$REPORT_DIR:$REPORT_DIR" \
  --mount type=tmpfs,destination=/work/repo/alt-frontend-sv/node_modules \
  -w /work/repo/alt-frontend-sv \
  -e CI=true \
  -e PW_NO_WEBSERVER=1 \
  -e E2E_FORWARDED_PROTO=http \
  -e ALT_E2E_PORT="$ALT_E2E_PORT" \
  -e FORCE_COLOR=1 \
  -e PLAYWRIGHT_SKIP_GIT_INFO=1 \
  -e PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
  -e npm_config_update_notifier=false \
  -e npm_config_fetch_retries=5 \
  -e npm_config_fetch_retry_maxtimeout=120000 \
  -e npm_config_fetch_timeout=600000 \
  -e NPM_CI_MAX_ATTEMPTS="$NPM_CI_MAX_ATTEMPTS" \
  -e NPM_CI_BACKOFF_SECS="$NPM_CI_BACKOFF_SECS" \
  -e PROJECTS="$PROJECTS" \
  -e SHARD="$SHARD" \
  -e REPORT_DIR="$REPORT_DIR" \
  -e PW_BLOB="${PW_BLOB:-}" \
  "$PLAYWRIGHT_IMAGE" \
  bash -euxo pipefail -c '
    cp /work/repo/e2e/playwright/package.json /work/repo/e2e/playwright/package-lock.json /work/

    for attempt in $(seq 1 "$NPM_CI_MAX_ATTEMPTS"); do
      if npm ci --prefix /work --no-audit --no-fund --ignore-scripts; then
        break
      fi
      if (( attempt < NPM_CI_MAX_ATTEMPTS )); then
        echo "==> npm ci attempt ${attempt}/${NPM_CI_MAX_ATTEMPTS} failed; retrying in ${NPM_CI_BACKOFF_SECS}s" >&2
        sleep "$NPM_CI_BACKOFF_SECS"
      else
        echo "==> npm ci exhausted ${NPM_CI_MAX_ATTEMPTS} attempts against the registry" >&2
        exit 1
      fi
    done

    # shellcheck disable=SC2086
    /work/node_modules/.bin/playwright test ${PROJECTS} --shard="${SHARD}"
  '

echo "==> suite passed. reports: $REPORT_DIR" >&2
