#!/usr/bin/env bash
# Manual deploy driver for the single-host Docker Compose stack.
#
# Intended workflow: run this after an ADR merges to main.
#
#   scripts/deploy.sh production               # normal path
#   scripts/deploy.sh --dry-run production     # print plan, touch nothing
#   scripts/deploy.sh --skip-verify production # emergency only — skips Pact gate
#   scripts/deploy.sh --only alt-backend production
#   scripts/deploy.sh --no-record production   # don't call record-deployment
#
# Pipeline:
#   pre-deploy-verify  →  build/pull  →  layered rolling recreate  →
#   global smoke  →  pact-broker record-deployment
# Any failure in recreate/smoke triggers a best-effort rollback to the previous SHA.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck disable=SC1091
source "$REPO_ROOT/scripts/_deploy_lib.sh"

COMPOSE_FILE="${COMPOSE_FILE:-$REPO_ROOT/compose/compose.yaml}"
PRE_DEPLOY_VERIFY_SCRIPT="${PRE_DEPLOY_VERIFY_SCRIPT:-$REPO_ROOT/scripts/pre-deploy-verify.sh}"
PACT_BROKER_BASE_URL="${PACT_BROKER_BASE_URL:-http://localhost:9292}"
PACT_BROKER_USERNAME="${PACT_BROKER_USERNAME:-pact}"
if [[ -z "${PACT_BROKER_PASSWORD:-}" ]]; then
  if [[ -r "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt" ]]; then
    PACT_BROKER_PASSWORD="$(tr -d '\n' < "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt")"
  fi
fi
export PACT_BROKER_BASE_URL PACT_BROKER_USERNAME PACT_BROKER_PASSWORD COMPOSE_FILE

DEPLOY_PACTICIPANTS="${DEPLOY_PACTICIPANTS:-alt-backend pre-processor search-indexer mq-hub rag-orchestrator recap-worker recap-subworker recap-evaluator news-creator tag-generator tts-speaker acolyte-orchestrator alt-butterfly-facade auth-hub}"
export DEPLOY_PACTICIPANTS
DEPLOY_REPO_ROOT="${DEPLOY_REPO_ROOT:-$REPO_ROOT}"
export DEPLOY_REPO_ROOT

DRY_RUN=0
SKIP_VERIFY=0
NO_RECORD=0
ONLY=""
TARGET_ENV=""

while (( $# > 0 )); do
  case "$1" in
    --dry-run) DRY_RUN=1; shift;;
    --skip-verify) SKIP_VERIFY=1; shift;;
    --no-record) NO_RECORD=1; shift;;
    --only) ONLY="$2"; shift 2;;
    --only=*) ONLY="${1#*=}"; shift;;
    -h|--help)
      sed -n '2,20p' "$0"
      exit 0 ;;
    production|staging|dev)
      TARGET_ENV="$1"; shift;;
    *)
      echo "unknown arg: $1" >&2
      exit 64 ;;
  esac
done
TARGET_ENV="${TARGET_ENV:-production}"
export DRY_RUN

VERSION="$(cd "$DEPLOY_REPO_ROOT" && git rev-parse --short HEAD 2>/dev/null || echo "local-$(date +%s)")"
echo "==> deploy  env=${TARGET_ENV}  version=${VERSION}  dry_run=${DRY_RUN}  only=${ONLY:-<all>}"

# --- 1. gate --------------------------------------------------------------
if (( SKIP_VERIFY == 1 )); then
  echo "==> skipping pre-deploy-verify (--skip-verify set)"
else
  echo "==> running pre-deploy-verify"
  if ! "$PRE_DEPLOY_VERIFY_SCRIPT"; then
    echo "pre-deploy-verify failed — deploy aborted." >&2
    exit 10
  fi
fi

# --- 2. record previous SHA for rollback ---------------------------------
if (( DRY_RUN == 0 )); then
  {
    echo "PREV_COMMIT=$(cd "$DEPLOY_REPO_ROOT" && git rev-parse HEAD 2>/dev/null)"
    echo "PREV_TIME=$(date -Iseconds)"
  } > "$DEPLOY_REPO_ROOT/.deploy-prev"
fi

# --- 3. pull / build ------------------------------------------------------
if (( DRY_RUN == 1 )); then
  echo "==> (dry-run) would pull / build images"
elif [[ -n "$ONLY" ]]; then
  echo "==> pulling image for ${ONLY}"
  "${DOCKER_BIN:-docker}" compose -f "$COMPOSE_FILE" pull "$ONLY" || true
else
  echo "==> pulling images"
  "${DOCKER_BIN:-docker}" compose -f "$COMPOSE_FILE" pull || true
fi

# --- 4. rolling recreate --------------------------------------------------
echo "==> rolling recreate"
LAYERS="$(resolve_layers)"
ANY_FAIL=0
FAILED_SVC=""
if [[ -n "$ONLY" ]]; then
  if ! deploy_single_service "$ONLY"; then
    ANY_FAIL=1; FAILED_SVC="$ONLY"
  fi
else
  while IFS= read -r svc; do
    [[ -z "$svc" ]] && { echo "  ---"; continue; }
    if ! deploy_single_service "$svc"; then
      ANY_FAIL=1; FAILED_SVC="$svc"
      break
    fi
  done <<<"$LAYERS"
fi

# --- 5. global smoke ------------------------------------------------------
if (( ANY_FAIL == 0 )); then
  echo "==> global smoke"
  if ! global_smoke; then
    ANY_FAIL=1; FAILED_SVC="<smoke>"
  fi
fi

# --- 6. rollback on failure ----------------------------------------------
if (( ANY_FAIL == 1 )); then
  echo "deploy failed at ${FAILED_SVC} — initiating rollback" >&2
  rollback_to_previous || true
  exit 11
fi

# --- 7. record-deployment -------------------------------------------------
if (( DRY_RUN == 1 )); then
  echo "==> (dry-run) skipping record-deployment"
elif (( NO_RECORD == 1 )); then
  echo "==> skipping record-deployment (--no-record set)"
else
  echo "==> recording deployment to Pact Broker"
  record_deployments "$VERSION" "$TARGET_ENV"
fi

echo ""
echo "==> deploy complete  env=${TARGET_ENV}  version=${VERSION}"
