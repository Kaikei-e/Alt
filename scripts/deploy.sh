#!/usr/bin/env bash
# Manual deploy driver for the single-host Docker Compose stack.
# Run after an ADR merges to main:
#
#   scripts/deploy.sh production
#
# Pipeline:
#   1. scripts/check-compose-variables.sh         (env render gate, touches nothing)
#   2. scripts/pact-check.sh --broker            (publishes pacts to the Broker)
#   3. scripts/retire-alt-pki-agent-leftovers.sh (project=alt pki-agent stop/rm + zero gate)
#   4. c2quay deploy --env <env> --config c2quay.yml
#        → can-i-deploy gate × 13 pacticipants (parallel, HAL)
#        → docker compose up -d --wait
#        → scripts/smoke.sh
#        → record-deployment × 13 (including gate_only services -- ADR 0013)
#
# Wave 4 retired scripts/cascade-pki-sidecars.sh. There are zero
# `network_mode: service:` pki sidecars; inbound TLS lives in the parent
# process. Do not re-add a cascade step. Leftover pki-agent-* containers
# are dual writers: sweep them with exact compose labels before parent up.
# Do not use `up --remove-orphans` as leftover cleanup (ADR-000978).
# matching pki-agent=0 is a steady no-op when project=alt anchors are
# visible; a completely empty listing is not a fresh install unless
# ALT_ACK_FRESH_INSTALL=1.
#
# Any step failing aborts the chain. Recovery is manual: git revert → re-commit
# → re-run this script. See docs/runbooks/deploy.md and
# docs/runbooks/pki-agent-recovery.md for details.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

usage() {
  cat <<'EOF'
Usage: scripts/deploy.sh [production|staging]
       scripts/deploy.sh --help

Pipeline (aborts on the first failure):
  1. check-compose-variables
  2. pact-check.sh --broker
  3. retire leftover pki-agent-* (project=alt exact labels, before parent up)
  4. c2quay deploy

Environment:
  ALT_ACK_FRESH_INSTALL=1
      Required only when no com.docker.compose.project=alt containers
      are visible. matching pki-agent=0 with live parents / other
      project anchors is a steady no-op and does not need this.
      Wrong Docker context or rootless vs root daemon looks empty —
      do not set this unless the host has never run project=alt.

  First install on a genuinely empty host:
      ALT_ACK_FRESH_INSTALL=1 ./scripts/deploy.sh production
EOF
}

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
esac

TARGET_ENV="${1:-production}"

export PACT_BROKER_USERNAME="${PACT_BROKER_USERNAME:-pact}"
if [[ -z "${PACT_BROKER_PASSWORD:-}" ]] && [[ -r "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt" ]]; then
  PACT_BROKER_PASSWORD="$(tr -d '\n' < "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt")"
fi
export PACT_BROKER_PASSWORD

PACT_CHECK_SCRIPT="${PACT_CHECK_SCRIPT:-$REPO_ROOT/scripts/pact-check.sh}"
C2QUAY_BIN="${C2QUAY_BIN:-c2quay}"
C2QUAY_CONFIG="${C2QUAY_CONFIG:-$REPO_ROOT/c2quay.yml}"
COMPOSE_VARS_SCRIPT="${COMPOSE_VARS_SCRIPT:-$REPO_ROOT/scripts/check-compose-variables.sh}"
LEFTOVER_SWEEP_SCRIPT="${LEFTOVER_SWEEP_SCRIPT:-$REPO_ROOT/scripts/retire-alt-pki-agent-leftovers.sh}"

# First, because an incomplete .env renders services with empty-string config
# and c2quay would happily deploy that (PM-2026-048).
echo "==> [1/4] check-compose-variables"
"$COMPOSE_VARS_SCRIPT"

echo "==> [2/4] pact-check.sh --broker"
"$PACT_CHECK_SCRIPT" --broker

# Before any PKI_ENROLLMENT=enabled parent up. Visible project anchors
# with zero pki-agent sidecars are a steady no-op. An empty listing
# aborts unless ALT_ACK_FRESH_INSTALL=1. Fail-closed if leftovers remain.
echo "==> [3/4] retire leftover pki-agent sidecars (project=alt, before parent up)"
"$LEFTOVER_SWEEP_SCRIPT"

echo "==> [4/4] c2quay deploy --env ${TARGET_ENV}"
"$C2QUAY_BIN" deploy --env "$TARGET_ENV" --config "$C2QUAY_CONFIG"

echo ""
echo "==> deploy complete  env=${TARGET_ENV}"
