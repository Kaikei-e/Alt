#!/usr/bin/env bash
# Manual deploy driver for the single-host Docker Compose stack.
# Run after an ADR merges to main:
#
#   scripts/deploy.sh production
#
# Pipeline:
#   1. scripts/pact-check.sh --broker            (publishes pacts to the Broker)
#   2. c2quay deploy --env <env> --config c2quay.yml
#        → can-i-deploy gate × 13 pacticipants (parallel, HAL)
#        → docker compose up -d --wait --remove-orphans
#        → scripts/smoke.sh
#        → record-deployment × 13
#   3. scripts/cascade-pki-sidecars.sh            (netns-sharing sidecar cascade; closes PM-2026-030 gap)
#   4. scripts/record-remote-pacticipant.sh <env>   (tts-speaker, remote GPU host)
#
# Any step failing aborts the chain. Recovery is manual: git revert → re-commit
# → re-run this script. See docs/runbooks/deploy.md for details.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

TARGET_ENV="${1:-production}"

export PACT_BROKER_USERNAME="${PACT_BROKER_USERNAME:-pact}"
if [[ -z "${PACT_BROKER_PASSWORD:-}" ]] && [[ -r "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt" ]]; then
  PACT_BROKER_PASSWORD="$(tr -d '\n' < "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt")"
fi
export PACT_BROKER_PASSWORD

PACT_CHECK_SCRIPT="${PACT_CHECK_SCRIPT:-$REPO_ROOT/scripts/pact-check.sh}"
C2QUAY_BIN="${C2QUAY_BIN:-c2quay}"
C2QUAY_CONFIG="${C2QUAY_CONFIG:-$REPO_ROOT/c2quay.yml}"
CASCADE_SCRIPT="${CASCADE_SCRIPT:-$REPO_ROOT/scripts/cascade-pki-sidecars.sh}"
RECORD_REMOTE_SCRIPT="${RECORD_REMOTE_SCRIPT:-$REPO_ROOT/scripts/record-remote-pacticipant.sh}"

echo "==> [1/4] pact-check.sh --broker"
"$PACT_CHECK_SCRIPT" --broker

echo "==> [2/4] c2quay deploy --env ${TARGET_ENV}"
"$C2QUAY_BIN" deploy --env "$TARGET_ENV" --config "$C2QUAY_CONFIG"

echo "==> [3/4] cascade-pki-sidecars"
"$CASCADE_SCRIPT"

echo "==> [4/4] record-remote-pacticipant (tts-speaker)"
"$RECORD_REMOTE_SCRIPT" "$TARGET_ENV"

echo ""
echo "==> deploy complete  env=${TARGET_ENV}"
