#!/bin/bash
# Alt ローカルデプロイスクリプト
# Usage: ./deploy-system/deploy-local.sh [--all | stack1 stack2 ...]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

LOG_FILE="${PROJECT_ROOT}/deploy-system/deploy.log"

log() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] $1"
    echo "$msg"
    echo "$msg" >> "$LOG_FILE"
}

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

log "Rebuilding and restarting services..."
if [ $# -eq 0 ]; then
    altctl up --build
else
    altctl up "$@" --build
fi

log "Running smoke tests..."
if bash "$SCRIPT_DIR/smoke-test.sh"; then
    log "=== Deploy complete ==="
else
    log "WARNING: Smoke tests failed after deploy"
    exit 1
fi
