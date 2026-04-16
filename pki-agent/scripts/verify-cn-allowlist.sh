#!/bin/bash
# Verifies F-001 / F-002 remediation:
#   1. pki-agent JWK provisioner exists in step-ca ca.json (separate from bootstrap)
#   2. Authority-level X.509 policy allowlists the 8 east-west subjects
#   3. CA rejects cert requests for a non-allowlisted CN (attacker.local)
#   4. CA accepts cert requests for an allowlisted CN (alt-backend)
#
# Open-source step-ca supports policy only at authority level (not per-provisioner)
# per https://smallstep.com/docs/step-ca/policies/ — so the allowlist applies to
# ALL provisioners, shrinking bootstrap's blast radius as well.

set -u

STEP_CA="${STEP_CA:-alt-step-ca-1}"
CA_CONFIG="${CA_CONFIG:-/home/step/config/ca.json}"
EXPECTED_CNS=(
  alt-backend
  alt-butterfly-facade
  auth-hub
  pre-processor
  search-indexer
  tag-generator
  recap-worker
  acolyte-orchestrator
  localhost
)

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

echo "=== Assertion 1: pki-agent provisioner exists ==="
assert "pki-agent provisioner present in ca.json" \
  docker exec "$STEP_CA" sh -c "apk add --no-cache jq >/dev/null 2>&1 || true; jq -e '.authority.provisioners[] | select(.name==\"pki-agent\" and .type==\"JWK\")' $CA_CONFIG"

echo "=== Assertion 2: authority.policy.x509 allowlist contains 9 names ==="
for cn in "${EXPECTED_CNS[@]}"; do
  assert "allow.dns contains '$cn'" \
    docker exec "$STEP_CA" sh -c "jq -e --arg cn '$cn' '(.authority.policy.x509.allow.dns // []) | index(\$cn)' $CA_CONFIG"
done

echo "=== Assertion 3: CA rejects cert request for attacker.local ==="
# Run from inside an existing pki-agent container so step-cli has the trust
# bundle, CA password secret, and network routing. Policy enforcement happens
# at cert-sign time, not OTT mint time.
if docker exec alt-pki-agent-alt-backend-1 sh -c '
      TOKEN=$(step ca token attacker.local \
        --ca-url https://step-ca:9000 \
        --root /trust/ca-bundle.pem \
        --provisioner pki-agent \
        --password-file /run/secrets/step_ca_root_password \
        --san attacker.local --force 2>/dev/null)
      step ca certificate attacker.local /tmp/c.pem /tmp/k.pem \
        --ca-url https://step-ca:9000 \
        --root /trust/ca-bundle.pem \
        --token "$TOKEN" --force 2>&1
    ' 2>&1 | grep -qiE "not allowed|forbidden|denied|policy"; then
  echo "PASS  CA rejects attacker.local with policy error"
  pass=$((pass + 1))
else
  echo "FAIL  CA did not clearly reject attacker.local via policy"
  fail=$((fail + 1))
fi

echo "=== Assertion 4: CA rejects non-DNS SAN types (IP/URI/email) ==="
# step-ca policy is deny-by-default for name types not listed in allow.
# Verify this behaviour so bypass via alternate SAN type is impossible.
for san in "10.0.0.99" "https://evil.com" "attacker@evil.com"; do
  if docker exec alt-pki-agent-alt-backend-1 sh -c "
        TOKEN=\$(step ca token alt-backend \
          --ca-url https://step-ca:9000 --root /trust/ca-bundle.pem \
          --provisioner pki-agent --password-file /run/secrets/step_ca_root_password \
          --san '$san' --force 2>/dev/null | tail -1)
        step ca certificate alt-backend /tmp/c.pem /tmp/k.pem \
          --ca-url https://step-ca:9000 --root /trust/ca-bundle.pem \
          --token \"\$TOKEN\" --force 2>&1
      " 2>&1 | grep -qiE "not allowed|forbidden|denied|policy"; then
    echo "PASS  CA rejects SAN '$san'"
    pass=$((pass + 1))
  else
    echo "FAIL  CA did not reject SAN '$san' (potential bypass)"
    fail=$((fail + 1))
  fi
done

echo "=== Assertion 5: CA accepts cert request for alt-backend (smoke) ==="
# Verify that the pki-agent sidecar for alt-backend is healthy — it issues with
# the pki-agent provisioner and must succeed.
assert "alt-backend pki-agent is healthy" \
  docker exec alt-pki-agent-alt-backend-1 wget -qO- http://127.0.0.1:9510/healthz

echo
echo "Summary: $pass passed, $fail failed"
exit "$fail"
