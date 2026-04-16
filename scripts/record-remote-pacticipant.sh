#!/usr/bin/env bash
# record-deployment for pacticipants that live off this host and therefore
# cannot be rolled out by c2quay. Currently this is tts-speaker (separate GPU
# host). Run at the tail of scripts/deploy.sh so the broker matrix reflects
# the intended release even when c2quay skipped the service.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEPLOY_REPO_ROOT="${DEPLOY_REPO_ROOT:-$REPO_ROOT}"

TARGET_ENV="${1:-production}"

PACT_BROKER_BIN="${PACT_BROKER_BIN:-pact-broker-cli}"
PACT_BROKER_BASE_URL="${PACT_BROKER_BASE_URL:-http://localhost:9292}"
PACT_BROKER_USERNAME="${PACT_BROKER_USERNAME:-pact}"
if [[ -z "${PACT_BROKER_PASSWORD:-}" ]] && [[ -r "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt" ]]; then
  PACT_BROKER_PASSWORD="$(tr -d '\n' < "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt")"
fi

VERSION="$(cd "$DEPLOY_REPO_ROOT" && git rev-parse --short HEAD)"

REMOTE_PACTICIPANTS=("tts-speaker")

fail=0
for svc in "${REMOTE_PACTICIPANTS[@]}"; do
  echo "==> record-deployment ${svc} @ ${VERSION} → ${TARGET_ENV}"
  if ! "$PACT_BROKER_BIN" record-deployment \
        --pacticipant "$svc" \
        --version "$VERSION" \
        --environment "$TARGET_ENV" \
        --broker-base-url "$PACT_BROKER_BASE_URL" \
        --broker-username "$PACT_BROKER_USERNAME" \
        --broker-password "$PACT_BROKER_PASSWORD"; then
    echo "   FAILED" >&2
    fail=$((fail + 1))
  fi
done

(( fail == 0 ))
