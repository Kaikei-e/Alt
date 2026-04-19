#!/bin/bash
# Idempotent one-time setup for the dedicated `pki-agent` JWK provisioner and
# authority-level X.509 CN allowlist in step-ca.
#
# Applies the F-001 / F-002 remediation from
# docs/review/pki-agent-security-audit-2026-04-16.md.
#
# Open-source step-ca supports policy only at authority level, so the allowlist
# applies to both `bootstrap` and `pki-agent` provisioners. This shrinks the
# blast radius of either provisioner being compromised.
#
# Run manually on a fresh install, or re-run safely (idempotent — skips if
# already configured). For existing installs where pki-agent provisioner is
# absent, step-ca will be SIGHUP'd so the new config loads without downtime.
#
# Usage: bash pki-agent/scripts/bootstrap-pki-provisioner.sh

set -euo pipefail

STEP_CA="${STEP_CA:-alt-step-ca-1}"
CA_CONFIG=/home/step/config/ca.json
PASSWORD_FILE=/run/secrets/step_ca_root_password

SUBJECTS=(
  alt-backend
  alt-butterfly-facade
  auth-hub
  pre-processor
  search-indexer
  tag-generator
  recap-worker
  acolyte-orchestrator
  recap-subworker
  news-creator
  localhost
)

# jq array literal for SUBJECTS
build_subjects_json() {
  printf '['
  local first=1
  for s in "${SUBJECTS[@]}"; do
    if [ $first -eq 1 ]; then first=0; else printf ','; fi
    printf '"%s"' "$s"
  done
  printf ']'
}

if ! docker ps --format '{{.Names}}' | grep -q "^${STEP_CA}$"; then
  echo "ERROR: ${STEP_CA} is not running. Start with 'docker compose up -d step-ca' first." >&2
  exit 1
fi

echo "==> ensuring jq is available inside ${STEP_CA}..."
docker exec -u 0 "$STEP_CA" sh -c 'command -v jq >/dev/null || apk add --no-cache jq' >/dev/null

echo "==> checking current provisioner state..."
if docker exec -u 0 "$STEP_CA" jq -e '.authority.provisioners[]? | select(.name=="pki-agent" and .type=="JWK")' "$CA_CONFIG" >/dev/null 2>&1; then
  echo "    pki-agent JWK provisioner already present."
  PROVISIONER_ADDED=0
else
  echo "==> adding pki-agent JWK provisioner (offline, --ca-config)..."
  docker exec -u 0 "$STEP_CA" step ca provisioner add pki-agent \
    --type JWK \
    --create \
    --password-file "$PASSWORD_FILE" \
    --ca-config "$CA_CONFIG"
  PROVISIONER_ADDED=1
fi

echo "==> checking authority.policy state..."
EXPECTED_POLICY=$(build_subjects_json)
CURRENT_DNS=$(docker exec -u 0 "$STEP_CA" jq -c '.authority.policy.x509.allow.dns // []' "$CA_CONFIG")
if [ "$CURRENT_DNS" = "$EXPECTED_POLICY" ]; then
  echo "    authority.policy.x509 already matches expected allowlist."
  POLICY_UPDATED=0
else
  echo "==> injecting authority-level X.509 CN/DNS allowlist..."
  docker exec -u 0 "$STEP_CA" sh -c "
    jq '.authority.policy = {
      \"x509\": {
        \"allow\": {
          \"dns\": $EXPECTED_POLICY,
          \"cn\":  $EXPECTED_POLICY
        },
        \"allowWildcardNames\": false
      }
    }' $CA_CONFIG > /tmp/ca.json.new && mv /tmp/ca.json.new $CA_CONFIG && chown step:step $CA_CONFIG
  "
  POLICY_UPDATED=1
fi

if [ "$PROVISIONER_ADDED" -eq 1 ] || [ "$POLICY_UPDATED" -eq 1 ]; then
  echo "==> sending SIGHUP to step-ca to reload config..."
  docker kill -s SIGHUP "$STEP_CA" >/dev/null
  sleep 2
  echo "    reload sent. Verify with: docker compose logs step-ca --tail 5"
else
  echo "==> nothing to do (already idempotent)."
fi

echo
echo "==> verification run..."
bash "$(dirname "$0")/verify-cn-allowlist.sh"
