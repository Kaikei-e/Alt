#!/usr/bin/env bash
# Local pre-deploy Pact gate. Runs before `scripts/deploy.sh` on the single host.
#
#   1. Pact Broker heartbeat
#   2. create-environment (idempotent — tolerates "already exists")
#   3. scripts/pact-check.sh --broker   (all consumer + provider tests)
#   4. pact-broker can-i-deploy for every pacticipant against --target-env
#
# Exit non-zero on any failure. Designed to be called manually or from deploy.sh.
#
# Environment overrides (mostly for tests):
#   PACT_BROKER_BASE_URL   default http://localhost:9292
#   PACT_BROKER_USERNAME   default pact
#   PACT_BROKER_PASSWORD   falls back to secrets/pact_broker_basic_auth_password.txt
#   TARGET_ENV             default production
#   PACT_CHECK_SCRIPT      default $REPO_ROOT/scripts/pact-check.sh
#   PACT_BROKER_BIN        default pact-broker (must be on PATH)
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

PACT_BROKER_BASE_URL="${PACT_BROKER_BASE_URL:-http://localhost:9292}"
PACT_BROKER_USERNAME="${PACT_BROKER_USERNAME:-pact}"
if [[ -z "${PACT_BROKER_PASSWORD:-}" ]]; then
  if [[ -r "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt" ]]; then
    PACT_BROKER_PASSWORD="$(tr -d '\n' < "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt")"
  else
    PACT_BROKER_PASSWORD="pact"
  fi
fi
TARGET_ENV="${TARGET_ENV:-production}"
PACT_CHECK_SCRIPT="${PACT_CHECK_SCRIPT:-$REPO_ROOT/scripts/pact-check.sh}"
PACT_BROKER_BIN="${PACT_BROKER_BIN:-pact-broker}"

PACTICIPANTS=(
  alt-backend
  pre-processor
  search-indexer
  mq-hub
  rag-orchestrator
  recap-worker
  recap-subworker
  recap-evaluator
  news-creator
  tag-generator
  tts-speaker
  acolyte-orchestrator
  alt-butterfly-facade
  auth-hub
)

VERSION="$(cd "$REPO_ROOT" && git rev-parse --short HEAD 2>/dev/null || echo "local-$(date +%s)")"

echo "==> pre-deploy-verify  target=${TARGET_ENV}  version=${VERSION}"

# --- 1. heartbeat --------------------------------------------------------
echo "--- 1/4 Pact Broker heartbeat ---"
if ! curl -fsS --max-time 5 \
      -u "${PACT_BROKER_USERNAME}:${PACT_BROKER_PASSWORD}" \
      "${PACT_BROKER_BASE_URL}/diagnostic/status/heartbeat" >/dev/null 2>&1; then
  echo "ERROR: Pact Broker heartbeat failed at ${PACT_BROKER_BASE_URL}." >&2
  echo "       Ensure the broker is running (docker compose up -d pact-broker)." >&2
  exit 2
fi
echo "broker reachable."

# --- 2. create-environment (idempotent) ----------------------------------
echo "--- 2/4 Ensuring environment '${TARGET_ENV}' exists ---"
# pact-broker seeds 'test' and 'production' automatically, but we call create-environment
# defensively for custom targets. Non-zero exit is treated as "already exists".
"$PACT_BROKER_BIN" create-environment \
  --name "${TARGET_ENV}" \
  --production \
  --broker-base-url "${PACT_BROKER_BASE_URL}" \
  --broker-username "${PACT_BROKER_USERNAME}" \
  --broker-password "${PACT_BROKER_PASSWORD}" >/dev/null 2>&1 || \
  echo "(environment '${TARGET_ENV}' already exists or create was skipped — continuing)"

# --- 3. pact-check (consumer + provider) ---------------------------------
echo "--- 3/4 Running pact-check.sh --broker ---"
if ! "$PACT_CHECK_SCRIPT" --broker; then
  echo "ERROR: contract regression — pact-check.sh --broker failed." >&2
  exit 3
fi

# --- 4. can-i-deploy per pacticipant -------------------------------------
echo "--- 4/4 can-i-deploy × ${#PACTICIPANTS[@]} pacticipants → ${TARGET_ENV} ---"
FAIL=0
FAILED_LIST=()
for svc in "${PACTICIPANTS[@]}"; do
  echo "  - ${svc} @ ${VERSION}"
  if "$PACT_BROKER_BIN" can-i-deploy \
        --pacticipant "${svc}" \
        --version "${VERSION}" \
        --to-environment "${TARGET_ENV}" \
        --broker-base-url "${PACT_BROKER_BASE_URL}" \
        --broker-username "${PACT_BROKER_USERNAME}" \
        --broker-password "${PACT_BROKER_PASSWORD}" >/dev/null 2>&1; then
    echo "    ok"
  else
    FAIL=$((FAIL + 1))
    FAILED_LIST+=("${svc}")
    echo "    FAILED"
  fi
done

if (( FAIL > 0 )); then
  echo "" >&2
  echo "Release blocked — ${FAIL} pacticipant(s) failed can-i-deploy: ${FAILED_LIST[*]}" >&2
  exit 4
fi

echo ""
echo "All ${#PACTICIPANTS[@]} pacticipants are safe to deploy to ${TARGET_ENV}."
