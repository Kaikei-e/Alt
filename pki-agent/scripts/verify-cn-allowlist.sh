#!/bin/bash
# Verifies F-001 / F-002 / Wave 4 subject-scoped enrollment:
#   1. Each workload CERT_SUBJECT has its own JWK provisioner
#      (pki-agent-<subject>), not a single shared JWK
#   2. Authority-level X.509 policy allowlists every east-west subject
#      (see EXPECTED_CNS below; kept in lockstep with the bootstrap script)
#   3. CA rejects cert requests for a non-allowlisted CN (attacker.local)
#   4. CA rejects non-DNS SAN types
#   5. CA accepts an allowlisted CN (alt-backend) via its own provisioner
#
# Open-source step-ca supports policy only at authority level (not per-provisioner)
# per https://smallstep.com/docs/step-ca/policies/ — so the allowlist applies to
# ALL provisioners, shrinking bootstrap's blast radius as well.
#
# Live docker/step-ca is required to run this script. Unit tests parse the
# source only and must not execute it.

set -u

STEP_CA="${STEP_CA:-alt-step-ca-1}"
CA_CONFIG="${CA_CONFIG:-/home/step/config/ca.json}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SECRET_DIR="${PKI_PROVISIONER_SECRET_DIR:-$(cd "${SCRIPT_DIR}/../../secrets" && pwd)}"

EXPECTED_CNS=(
  alt-backend
  # Keep in lockstep with SUBJECTS in bootstrap-pki-provisioner.sh.
  alt-harvester
  alt-data-hub
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

fail=0
pass=0

assert() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    echo "PASS  $name"
    pass=$((pass + 1))
  else
    echo "FAIL  $name"
    fail=$((fail + 1))
  fi
}

echo "=== Assertion 1: per-subject JWK provisioners exist ==="
for cn in "${EXPECTED_CNS[@]}"; do
  if [ "$cn" = "localhost" ]; then
    continue
  fi
  name="$(provisioner_name_for "$cn")"
  assert "JWK provisioner '$name' present in ca.json" \
    docker exec "$STEP_CA" sh -c "apk add --no-cache jq >/dev/null 2>&1 || true; jq -e --arg n '$name' '.authority.provisioners[] | select(.name==\$n and .type==\"JWK\")' $CA_CONFIG"
done

echo "=== Assertion 2: authority.policy.x509 allowlist contains all expected names ==="
for cn in "${EXPECTED_CNS[@]}"; do
  assert "allow.dns contains '$cn'" \
    docker exec "$STEP_CA" sh -c "jq -e --arg cn '$cn' '(.authority.policy.x509.allow.dns // []) | index(\$cn)' $CA_CONFIG"
done

SMOKE_SUBJECT=alt-backend
SMOKE_PROVISIONER="$(provisioner_name_for "$SMOKE_SUBJECT")"
SMOKE_PW_HOST="$(host_password_file_for "$SMOKE_SUBJECT")"
SMOKE_PW_CA="/tmp/${SMOKE_PROVISIONER}.verify.pw"

install_smoke_password() {
  if [ ! -s "$SMOKE_PW_HOST" ]; then
    echo "FAIL  missing host provisioner password for ${SMOKE_SUBJECT} (run bootstrap first)"
    fail=$((fail + 1))
    return 1
  fi
  docker exec -i -u 0 "$STEP_CA" sh -c "cat > '$SMOKE_PW_CA' && chmod 400 '$SMOKE_PW_CA'" < "$SMOKE_PW_HOST"
}

cleanup_smoke_password() {
  docker exec -u 0 "$STEP_CA" rm -f "$SMOKE_PW_CA" >/dev/null 2>&1 || true
}

echo "=== Assertion 3: CA rejects cert request for attacker.local ==="
# Mint inside step-ca with the subject-scoped JWK. Do not use the CA root
# password, and do not print token/password bytes. `-u 0` because
# install_smoke_password writes the password file as root with mode 400 —
# the image's default `step` user cannot read it, and `step ca token` then
# fails for a reason that has nothing to do with the policy under test.
if install_smoke_password; then
  if docker exec -u 0 "$STEP_CA" sh -c "
        TOKEN=\$(step ca token attacker.local \
          --ca-url https://localhost:9000 \
          --root /home/step/certs/root_ca.crt \
          --provisioner '$SMOKE_PROVISIONER' \
          --password-file '$SMOKE_PW_CA' \
          --san attacker.local --force 2>/dev/null)
        step ca certificate attacker.local /tmp/c.pem /tmp/k.pem \
          --ca-url https://localhost:9000 \
          --root /home/step/certs/root_ca.crt \
          --token \"\$TOKEN\" --force 2>&1
      " 2>&1 | grep -qiE "not allowed|forbidden|denied|policy"; then
    echo "PASS  CA rejects attacker.local with policy error"
    pass=$((pass + 1))
  else
    echo "FAIL  CA did not clearly reject attacker.local via policy"
    fail=$((fail + 1))
  fi
fi

echo "=== Assertion 4: CA rejects non-DNS SAN types (IP/URI/email) ==="
# step-ca policy is deny-by-default for name types not listed in allow.
for san in "10.0.0.99" "https://evil.com" "attacker@evil.com"; do
  if docker exec -u 0 "$STEP_CA" sh -c "
        TOKEN=\$(step ca token $SMOKE_SUBJECT \
          --ca-url https://localhost:9000 --root /home/step/certs/root_ca.crt \
          --provisioner '$SMOKE_PROVISIONER' --password-file '$SMOKE_PW_CA' \
          --san '$san' --force 2>/dev/null | tail -1)
        step ca certificate $SMOKE_SUBJECT /tmp/c.pem /tmp/k.pem \
          --ca-url https://localhost:9000 --root /home/step/certs/root_ca.crt \
          --token \"\$TOKEN\" --force 2>&1
      " 2>&1 | grep -qiE "not allowed|forbidden|denied|policy"; then
    echo "PASS  CA rejects SAN '$san'"
    pass=$((pass + 1))
  else
    echo "FAIL  CA did not reject SAN '$san' (potential bypass)"
    fail=$((fail + 1))
  fi
done

echo "=== Assertion 5: CA accepts cert request for ${SMOKE_SUBJECT} (smoke) ==="
if docker exec -u 0 "$STEP_CA" sh -c "
      TOKEN=\$(step ca token $SMOKE_SUBJECT \
        --ca-url https://localhost:9000 --root /home/step/certs/root_ca.crt \
        --provisioner '$SMOKE_PROVISIONER' --password-file '$SMOKE_PW_CA' \
        --san $SMOKE_SUBJECT --force 2>/dev/null | tail -1)
      step ca certificate $SMOKE_SUBJECT /tmp/ok-c.pem /tmp/ok-k.pem \
        --ca-url https://localhost:9000 --root /home/step/certs/root_ca.crt \
        --token \"\$TOKEN\" --force
    " >/dev/null 2>&1; then
  echo "PASS  CA signs ${SMOKE_SUBJECT} with its own provisioner"
  pass=$((pass + 1))
else
  echo "FAIL  CA did not sign ${SMOKE_SUBJECT} with ${SMOKE_PROVISIONER}"
  fail=$((fail + 1))
fi
cleanup_smoke_password

echo
echo "Summary: $pass passed, $fail failed"
exit "$fail"
