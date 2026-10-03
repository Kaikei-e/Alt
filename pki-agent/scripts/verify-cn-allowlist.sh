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
# Every subject-scoped provisioner carries an X.509 template (written by
# bootstrap-pki-provisioner.sh) that rejects first, so assertions 3/4 expect
# the template's message ("Invalid OTT claims: sub mismatch" / "Invalid OTT
# claims: unapproved SAN") and pass only when the request failed AND no
# certificate file was written. The authority-level policy is the backstop
# behind the template; assertion 2 checks its contents.
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
  tts-speaker
  knowledge-sovereign
  recap-evaluator
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

# Requests a certificate for subject $1 with SAN $2 through the smoke
# provisioner, inside step-ca. Prints the step output followed by one verdict
# line: REJECTED (non-zero exit and no cert file), ISSUED, or NO_TOKEN. The
# cert and key are removed before and after on every path, and the token and
# password never reach stdout. `-u 0` because install_smoke_password
# writes the password file as root with mode 400 — the image's default `step`
# user cannot read it, and the attempt would then fail for a reason that has
# nothing to do with the rejection under test.
attempt_issue() {
  local subject="$1" san="$2"
  docker exec -u 0 "$STEP_CA" sh -c "
    rm -f /tmp/c.pem /tmp/k.pem
    TOKEN=\$(step ca token '$subject' \
      --ca-url https://localhost:9000 --root /home/step/certs/root_ca.crt \
      --provisioner '$SMOKE_PROVISIONER' --password-file '$SMOKE_PW_CA' \
      --san '$san' --force 2>/dev/null | tail -1)
    if [ -z \"\$TOKEN\" ]; then
      echo NO_TOKEN
      exit 0
    fi
    step ca certificate '$subject' /tmp/c.pem /tmp/k.pem \
      --ca-url https://localhost:9000 --root /home/step/certs/root_ca.crt \
      --token \"\$TOKEN\" --force 2>&1
    rc=\$?
    if [ \$rc -ne 0 ] && [ ! -e /tmp/c.pem ]; then
      echo REJECTED
    else
      echo ISSUED
    fi
    rm -f /tmp/c.pem /tmp/k.pem
  " 2>&1
}

# True only when attempt_issue's final line is REJECTED and its output carries
# the expected template message. ISSUED, NO_TOKEN, a docker error, or a
# rejection for some other reason are all false.
rejected_with() {
  local output="$1" expected="$2"
  [ "$(printf '%s' "$output" | tail -n 1)" = "REJECTED" ] &&
    printf '%s' "$output" | grep -qF -- "$expected"
}

show_attempt() {
  printf '%s\n' "$1" |
    sed -E 's/eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*/<token>/g; s/^/      /'
}

echo "=== Assertion 3: CA rejects cert request for attacker.local ==="
# Mint inside step-ca with the subject-scoped JWK, never the CA root password.
if install_smoke_password; then
  out="$(attempt_issue attacker.local attacker.local)"
  if rejected_with "$out" "sub mismatch"; then
    echo "PASS  CA rejects attacker.local (template: sub mismatch, no cert issued)"
    pass=$((pass + 1))
  else
    echo "FAIL  CA did not reject attacker.local with 'sub mismatch'"
    show_attempt "$out"
    fail=$((fail + 1))
  fi
fi

echo "=== Assertion 4: CA rejects non-DNS SAN types (IP/URI/email) ==="
for san in "10.0.0.99" "https://evil.com" "attacker@evil.com"; do
  out="$(attempt_issue "$SMOKE_SUBJECT" "$san")"
  if rejected_with "$out" "unapproved SAN"; then
    echo "PASS  CA rejects SAN '$san' (template: unapproved SAN, no cert issued)"
    pass=$((pass + 1))
  else
    echo "FAIL  CA did not reject SAN '$san' with 'unapproved SAN' (potential bypass)"
    show_attempt "$out"
    fail=$((fail + 1))
  fi
done

echo "=== Assertion 5: CA accepts cert request for ${SMOKE_SUBJECT} (smoke) ==="
# attempt_issue deletes the signed leaf and its key again, so no usable
# credential is left behind in the CA container.
out="$(attempt_issue "$SMOKE_SUBJECT" "$SMOKE_SUBJECT")"
if [ "$(printf '%s' "$out" | tail -n 1)" = "ISSUED" ]; then
  echo "PASS  CA signs ${SMOKE_SUBJECT} with its own provisioner"
  pass=$((pass + 1))
else
  echo "FAIL  CA did not sign ${SMOKE_SUBJECT} with ${SMOKE_PROVISIONER}"
  show_attempt "$out"
  fail=$((fail + 1))
fi
cleanup_smoke_password

echo
echo "Summary: $pass passed, $fail failed"
exit "$fail"
