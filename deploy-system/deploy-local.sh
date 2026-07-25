#!/bin/bash
# Alt ローカルデプロイスクリプト
# Usage: ./deploy-system/deploy-local.sh [--all | service1 service2 ...]
#
# Deploy path (c2quay is the ONLY sanctioned deploy path — PM-2026-031
# forbids bypassing the Pact gate; `altctl deploy` has been removed):
#   1. git fetch/pull --ff-only
#   2. docker compose build     (c2quay itself is deliberately build-free,
#                                 pull:never — image builds stay here)
#   3. c2quay deploy --env production --config c2quay.yml
#        -> Pact can-i-deploy gate (all_or_nothing, incl. gate_only services)
#        -> docker compose up -d --wait --remove-orphans
#        -> scripts/smoke.sh (deploy.smoke in c2quay.yml)
#        -> record-deployment for every gated pacticipant
#   4. deploy-system/smoke-test.sh — supplementary checks c2quay's smoke
#      does not cover (see header comment in that script)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

LOG_FILE="${PROJECT_ROOT}/deploy-system/deploy.log"
COMPOSE_FILE="${PROJECT_ROOT}/compose/compose.yaml"
C2QUAY_CONFIG="${PROJECT_ROOT}/c2quay.yml"
C2QUAY_BIN="${C2QUAY_BIN:-c2quay}"

log() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] $1"
    echo "$msg"
    echo "$msg" >> "$LOG_FILE"
}

if ! command -v "$C2QUAY_BIN" >/dev/null 2>&1; then
    log "ERROR: c2quay binary not found (looked for '$C2QUAY_BIN' on PATH)."
    log "       Install it with: make install-c2quay"
    exit 1
fi

# compose.yaml's logging include requires DOCKER_GROUP_ID to even parse
# (`${DOCKER_GROUP_ID:?...}` in compose/logging.yaml) -- this hard-fails at
# config-parse time, before docker is touched at all, for the `docker
# compose ... build` step below. Fail fast here with the real remediation
# instead of letting compose's own opaque interpolation error be the first
# thing the operator sees.
if [ -z "${DOCKER_GROUP_ID:-}" ]; then
    log "ERROR: DOCKER_GROUP_ID is not set."
    log "       compose/logging.yaml requires it to parse the compose config."
    log "       Run: export DOCKER_GROUP_ID=\$(\"$PROJECT_ROOT/scripts/get-docker-gid.sh\")"
    exit 1
fi

# --all is kept only for backward-compatible invocation; docker compose
# build with no service args already builds every service, so it's a
# no-op once stripped.
BUILD_ARGS=()
for arg in "$@"; do
    if [ "$arg" != "--all" ]; then
        BUILD_ARGS+=("$arg")
    fi
done

# リモートの最新状態をチェック
git fetch origin main --quiet

LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse origin/main)

if [ "$LOCAL" = "$REMOTE" ]; then
    log "Already up to date ($(echo "$LOCAL" | head -c 8))"
    exit 0
fi

log "=== Alt Local Deploy ==="
log "Local:  $(echo "$LOCAL" | head -c 8)"
log "Remote: $(echo "$REMOTE" | head -c 8)"

log "Pulling latest changes..."
if ! git pull --ff-only origin main; then
    log "ERROR: git pull --ff-only failed. Manual intervention required."
    exit 1
fi

NEW_HEAD=$(git rev-parse HEAD)
log "Updated to $(echo "$NEW_HEAD" | head -c 8)"

log "Building images..."
if [ "${#BUILD_ARGS[@]}" -eq 0 ]; then
    if ! docker compose -f "$COMPOSE_FILE" -p alt build; then
        log "ERROR: docker compose build failed."
        exit 1
    fi
else
    if ! docker compose -f "$COMPOSE_FILE" -p alt build "${BUILD_ARGS[@]}"; then
        log "ERROR: docker compose build failed for: ${BUILD_ARGS[*]}"
        exit 1
    fi
fi

log "Deploying via c2quay (Pact gate + compose up --wait + smoke + record-deployment)..."
if "$C2QUAY_BIN" deploy --env production --config "$C2QUAY_CONFIG"; then
    log "c2quay deploy succeeded."
else
    log "ERROR: c2quay deploy failed (Pact gate, compose up, or smoke test). See output above."
    exit 1
fi

log "Running supplementary smoke checks..."
if bash "$SCRIPT_DIR/smoke-test.sh"; then
    log "=== Deploy complete ==="
else
    log "WARNING: supplementary smoke checks failed after deploy"
    exit 1
fi
