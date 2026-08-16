#!/usr/bin/env bash
# Manual deploy driver for the single-host Docker Compose stack.
# Run after an ADR merges to main:
#
#   scripts/deploy.sh production
#
# Pipeline:
#   1. scripts/check-compose-variables.sh         (env render gate, touches nothing)
#   2. scripts/pact-check.sh --broker            (publishes pacts to the Broker)
#   3. c2quay deploy --env <env> --config c2quay.yml
#        → can-i-deploy gate × 13 pacticipants (parallel, HAL)
#        → docker compose up -d --wait --remove-orphans
#        → scripts/smoke.sh
#        → record-deployment × 13 (including gate_only services -- ADR 0013)
#   4. scripts/cascade-pki-sidecars.sh            (netns-sharing sidecar cascade)
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
COMPOSE_VARS_SCRIPT="${COMPOSE_VARS_SCRIPT:-$REPO_ROOT/scripts/check-compose-variables.sh}"

# First, because an incomplete .env renders services with empty-string config
# and c2quay would happily deploy that (PM-2026-048).
echo "==> [1/4] check-compose-variables"
"$COMPOSE_VARS_SCRIPT"

echo "==> [2/4] pact-check.sh --broker"
"$PACT_CHECK_SCRIPT" --broker

echo "==> [3/4] c2quay deploy --env ${TARGET_ENV}"
"$C2QUAY_BIN" deploy --env "$TARGET_ENV" --config "$C2QUAY_CONFIG"

echo "==> [4/4] cascade-pki-sidecars"
"$CASCADE_SCRIPT"

echo ""
echo "==> deploy complete  env=${TARGET_ENV}"
