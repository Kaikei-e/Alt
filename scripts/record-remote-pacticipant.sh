#!/usr/bin/env bash
# record-deployment for pacticipants that live off this host and therefore
# cannot be rolled out by c2quay. Currently this is tts-speaker (separate GPU
# host). Run at the tail of scripts/deploy.sh so the broker matrix reflects
# the intended release even when c2quay skipped the service.
#
# NOTE (likely redundant as of the gate_only rollout): services.yaml now
# flags tts-speaker as `kind: external`, and scripts/regen-service-registry.py
# emits it into c2quay.yml with `gate_only: true`. c2quay natively includes
# gate_only services in the Pact can-i-deploy gate and runs record-deployment
# for them itself (see /workspace/c2quay docs/adr/0013-gate-only-services.md),
# without attempting to `docker compose up` them. That means c2quay deploy
# should already record tts-speaker's deployment, making this script's
# invocation in scripts/deploy.sh a duplicate in the common case. It is kept
# here as a manual fallback (see c2quay.yml's comment above the services
# block) in case gate_only is ever reverted for tts-speaker, or the broker
# record from a gate_only run needs to be repaired by hand. Do not delete.
set -euo pipefail

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
