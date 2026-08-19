#!/bin/bash
# Idempotent setup for *subject-scoped* JWK provisioners and the
# authority-level X.509 CN allowlist in step-ca.
#
# Wave 4 / F-001: one JWK provisioner per CERT_SUBJECT. Do not add a single
# shared JWK named for every sidecar. Do not use the CA root password as a
# workload provisioner password.
#
# Admin/root material stays here (bootstrap host + step-ca container). Workloads
# receive only their own `pki-agent-<subject>-jwk` secret.
#
# The shared historical JWK, if already present, is left in place so current
# compose sidecars keep working until cutover. This script never creates it.
#
# Usage: bash pki-agent/scripts/bootstrap-pki-provisioner.sh
#
# Secret files (gitignored under secrets/):
#   secrets/pki-agent-<subject>-jwk.txt
# In-container (compose cutover):
#   /run/secrets/pki-agent-<subject>-jwk

set -euo pipefail

STEP_CA="${STEP_CA:-alt-step-ca-1}"
CA_CONFIG=/home/step/config/ca.json
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SECRET_DIR="${PKI_PROVISIONER_SECRET_DIR:-$(cd "${SCRIPT_DIR}/../../secrets" && pwd)}"

SUBJECTS=(
  alt-backend
  # Split out of alt-backend: alt-harvester is an mTLS *client* (outbox-worker
  # -> rag-orchestrator, all jobs -> alt-data-hub:9443), alt-data-hub is the
  # mTLS-only data plane server. Both enroll in-process (Wave 4 first Go
  # cohort). A leftover pki-agent sidecar on the same cert volume is a dual
  # writer and is forbidden.
  alt-harvester
  alt-data-hub
  # The fourth binary of the same module: the Web Push dispatcher, an mTLS
  # client of alt-data-hub. Same sidecar shape, same hard dependency on a
  # signed leaf — di.newDataHubClient is fail-closed and panics on a missing
  # cert, so an unlisted CN here is a crash loop, not a degraded mode.
  alt-notifier
  alt-butterfly-facade
  auth-hub
  pre-processor
  search-indexer
  tag-generator
  recap-worker
  acolyte-orchestrator
  recap-subworker
  news-creator
  rag-orchestrator
  localhost
)

provisioner_name_for() {
  printf 'pki-agent-%s' "$1"
}

host_password_file_for() {
  printf '%s/pki-agent-%s-jwk.txt' "$SECRET_DIR" "$1"
}

# jq array literal for SUBJECTS (allowlist includes localhost; provisioners do not)
build_subjects_json() {
  printf '['
  local first=1
  for s in "${SUBJECTS[@]}"; do
    if [ $first -eq 1 ]; then first=0; else printf ','; fi
    printf '"%s"' "$s"
  done
  printf ']'
}

ensure_host_password_file() {
  local path="$1"
  if [ -s "$path" ]; then
    chmod 400 "$path" 2>/dev/null || true
    return 0
  fi
  umask 077
  mkdir -p "$(dirname "$path")"
  # Do not print the bytes. /dev/urandom keeps this off openssl's CLI args.
  head -c 32 /dev/urandom | base64 > "$path"
  chmod 400 "$path"
}

copy_password_into_ca() {
  local host_file="$1"
  local container_file="$2"
  # stdin is the secret; it is not echoed.
  docker exec -i -u 0 "$STEP_CA" sh -c "cat > '$container_file' && chmod 400 '$container_file'" < "$host_file"
}

if ! docker ps --format '{{.Names}}' | grep -q "^${STEP_CA}$"; then
  echo "ERROR: ${STEP_CA} is not running. Start with 'docker compose up -d step-ca' first." >&2
  exit 1
fi

echo "==> ensuring jq is available inside ${STEP_CA}..."
docker exec -u 0 "$STEP_CA" sh -c 'command -v jq >/dev/null || apk add --no-cache jq' >/dev/null

echo "==> adding per-subject JWK provisioners (offline, --ca-config)..."
PROVISIONER_ADDED=0
mkdir -p "$SECRET_DIR"
for subject in "${SUBJECTS[@]}"; do
  if [ "$subject" = "localhost" ]; then
    echo "    skip provisioner for allowlist-only name localhost"
    continue
  fi
  name="$(provisioner_name_for "$subject")"
  host_pw="$(host_password_file_for "$subject")"
  container_pw="/tmp/${name}.pw"
  ensure_host_password_file "$host_pw"
  if docker exec -u 0 "$STEP_CA" jq -e --arg n "$name" '.authority.provisioners[]? | select(.name==$n and .type=="JWK")' "$CA_CONFIG" >/dev/null 2>&1; then
    echo "    provisioner ${name} already present."
    continue
  fi
  echo "    adding provisioner ${name}"
  copy_password_into_ca "$host_pw" "$container_pw"
  docker exec -u 0 "$STEP_CA" step ca provisioner add "$name" \
    --type JWK \
    --create \
    --password-file "$container_pw" \
    --ca-config "$CA_CONFIG"
  docker exec -u 0 "$STEP_CA" rm -f "$container_pw"
  PROVISIONER_ADDED=1
done

echo "==> checking authority.policy state..."
EXPECTED_POLICY=$(build_subjects_json)
CURRENT_DNS=$(docker exec -u 0 "$STEP_CA" jq -c '.authority.policy.x509.allow.dns // []' "$CA_CONFIG")
POLICY_UPDATED=0
if [ "$CURRENT_DNS" = "$EXPECTED_POLICY" ]; then
  echo "    authority.policy.x509 already matches expected allowlist."
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
bash "${SCRIPT_DIR}/verify-cn-allowlist.sh"
