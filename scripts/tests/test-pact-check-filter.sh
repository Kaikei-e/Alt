#!/usr/bin/env bash
# Filter-behaviour tests for scripts/pact-check.sh.
#
# The script now supports three additive flags on top of the existing
# --services CSV filter:
#
#   --role consumer|provider   tighten matching to labels of that role
#   --dry-run                  print "WOULD RUN: <label>" per step, no exec
#   --publish-manual-verifications  run the bridging-evidence producers and
#                              the manual-verification block, nothing else
#
# The matrix-per-service refactor in alt-deploy depends on these
# semantics: each matrix leg must invoke pact-check.sh with its own
# service + role and see only its own step(s) run, with no accidental
# substring collisions (e.g. "search-indexer" filter pulling in
# "Go: mq-hub provider (search-indexer message pact)").

set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")"/.. && pwd)/pact-check.sh"
TESTS=0
FAILS=0

assert_contains() {
  local output="$1" needle="$2" desc="$3"
  TESTS=$((TESTS + 1))
  if grep -qF -- "$needle" <<<"$output"; then
    echo "  PASS: $desc"
  else
    echo "  FAIL: $desc"
    echo "    expected output to contain: $needle"
    FAILS=$((FAILS + 1))
  fi
}

assert_not_contains() {
  local output="$1" needle="$2" desc="$3"
  TESTS=$((TESTS + 1))
  if grep -qF -- "$needle" <<<"$output"; then
    echo "  FAIL: $desc"
    echo "    expected output NOT to contain: $needle"
    FAILS=$((FAILS + 1))
  else
    echo "  PASS: $desc"
  fi
}

run_script() {
  # Invoke pact-check.sh from the repo root so $REPO_ROOT resolves.
  # Errors inside the script bubble up via stdout+stderr combined.
  (cd "$(dirname "$SCRIPT")/.." && "$SCRIPT" "$@") 2>&1 || true
}

# --- search-indexer provider leg ---
echo "== --services search-indexer --role provider (dry-run) =="
out=$(run_script --dry-run --publish-only --services search-indexer --role provider)
assert_contains "$out" "WOULD RUN: Go: search-indexer provider" \
  "search-indexer provider leg runs its own step"
assert_not_contains "$out" "WOULD RUN: Go: mq-hub provider (search-indexer message pact)" \
  "search-indexer filter must not drag in mq-hub's message-pact provider step"
assert_not_contains "$out" "WOULD RUN: Go: search-indexer consumer" \
  "role=provider excludes the consumer step"
assert_not_contains "$out" "WOULD RUN: Rust: recap-worker" \
  "role=provider + service=search-indexer excludes unrelated recap-worker"

# --- recap-worker consumer leg (Rust) ---
echo "== --services recap-worker --role consumer (dry-run) =="
out=$(run_script --dry-run --publish-only --services recap-worker --role consumer)
assert_contains "$out" "WOULD RUN: Rust: recap-worker consumer" \
  "recap-worker consumer leg runs its own step"
assert_not_contains "$out" "WOULD RUN: Rust: recap-worker provider" \
  "role=consumer excludes the provider step"
assert_not_contains "$out" "WOULD RUN: Go: " \
  "recap-worker filter does not run any Go step"

# --- mq-hub provider leg: must include the message-pact step ---
echo "== --services mq-hub --role provider (dry-run) =="
out=$(run_script --dry-run --publish-only --services mq-hub --role provider)
assert_contains "$out" "WOULD RUN: Go: mq-hub provider" \
  "mq-hub provider leg runs the message-pact step"
assert_not_contains "$out" "WOULD RUN: Go: search-indexer provider" \
  "mq-hub provider leg does not accidentally run search-indexer provider"

# --- publish-manual-verifications runs the evidence producers + manual block ---
#
# The bridging block may only publish a verification record for a row whose
# verifier left evidence during this same invocation, so the dedicated leg
# has to run those verifiers. Everything else still stays out.
echo "== --publish-manual-verifications (dry-run, default filter) =="
out=$(run_script --dry-run --publish-only --publish-manual-verifications)
assert_contains "$out" "WOULD RUN: Rust: recap-worker provider" \
  "manual-verification subcommand runs the step that produces bridging evidence"
assert_not_contains "$out" "WOULD RUN: Go: alt-backend consumer" \
  "manual-verification subcommand does not run unit pact steps"
assert_not_contains "$out" "WOULD RUN: Python: news-creator provider" \
  "manual-verification subcommand does not run provider verifications that produce no evidence"

if command -v ansible-playbook >/dev/null 2>&1; then
  assert_contains "$out" "WOULD POST MANUAL VERIFICATION:" \
    "manual-verification subcommand announces the bridging records"
  assert_contains "$out" "[evidence: MISSING]" \
    "dry run reports the evidence verdict rather than assuming success"
  assert_contains "$out" "NO EVIDENCE: mq-hub -> tag-generator" \
    "the inverted tag-generator row is named as unverifiable, not silently skipped"
  assert_contains "$out" "NO EVIDENCE: auth-hub -> kratos" \
    "the external kratos row is named as unverifiable, not silently skipped"
else
  echo "  SKIP: ansible-playbook missing — bridging-plan assertions not run"
fi

# --- --skip-manual-bridge: opt out of the broker-side bridging block ---
# The deploy matrix runs ~20 publish legs per release and every one of them
# used to replay the whole bridging playbook, which a dedicated pact-manual
# leg already runs once. Opting out has to be explicit: inferring it from
# "a filter is set" would silently defang verify-pact-on-demand.yaml, which
# invokes --services without --role and depends on the bridge running.
echo "== --skip-manual-bridge (dry-run) =="
out=$(run_script --dry-run --publish-only --services alt-backend --role consumer --skip-manual-bridge)
assert_not_contains "$out" "Publishing Manual Verifications to Broker" \
  "--skip-manual-bridge suppresses the broker-side bridging block"
assert_contains "$out" "WOULD RUN: Go: alt-backend consumer" \
  "--skip-manual-bridge leaves the leg's own steps alone"

echo "== filtered leg without --skip-manual-bridge still bridges =="
out=$(run_script --dry-run --publish-only --services recap-worker)
assert_contains "$out" "Publishing Manual Verifications to Broker" \
  "a --services-only leg still bridges when the flag is absent"

echo "== --publish-manual-verifications ignores --skip-manual-bridge =="
out=$(run_script --dry-run --publish-only --publish-manual-verifications --skip-manual-bridge)
assert_contains "$out" "Publishing Manual Verifications to Broker" \
  "the dedicated bridging leg bridges even if the flag is passed"

# --- backward compat: --role omitted keeps the current substring behavior ---
echo "== --services alt-backend (no --role, backward compat) =="
out=$(run_script --dry-run --publish-only --services alt-backend)
assert_contains "$out" "WOULD RUN: Go: alt-backend consumer" \
  "no role: alt-backend matches consumer"
assert_contains "$out" "WOULD RUN: Go: alt-backend provider" \
  "no role: alt-backend matches provider"

# --- unfiltered dry-run visits every step ---
echo "== unfiltered (dry-run) =="
out=$(run_script --dry-run --publish-only)
assert_contains "$out" "WOULD RUN: Go: alt-backend consumer" "all-in: alt-backend consumer"
assert_contains "$out" "WOULD RUN: Rust: recap-worker consumer" "all-in: recap-worker consumer"
assert_contains "$out" "WOULD RUN: Python: recap-evaluator consumer" "all-in: recap-evaluator consumer"
assert_contains "$out" "WOULD RUN: Python: news-creator provider" "all-in: news-creator provider"

# --- publish filter: consumer leg owns only its own pacts ---
#
# The publish loop must be scoped to pact files whose .consumer.name matches
# the --services filter. Without this, parallel matrix legs all publish the
# full pact set, causing HTTP 409 Conflict on the Pact Broker when the same
# consumer_version gets two slightly-different bodies in a race.
# See docs/adr/.../pact-broker-409-remediation.md for background.
echo "== publish filter: recap-worker consumer leg (dry-run) =="
out=$(run_script --dry-run --publish-only --services recap-worker --role consumer)
assert_contains "$out" "WOULD PUBLISH: consumer=recap-worker" \
  "recap-worker consumer leg publishes its own pacts"
assert_not_contains "$out" "WOULD PUBLISH: consumer=recap-evaluator" \
  "recap-worker consumer leg must NOT publish recap-evaluator pacts (different owner)"
assert_not_contains "$out" "WOULD PUBLISH: consumer=alt-backend" \
  "recap-worker consumer leg must NOT publish alt-backend pacts"
assert_not_contains "$out" "WOULD PUBLISH: consumer=rag-orchestrator" \
  "recap-worker consumer leg must NOT publish rag-orchestrator pacts"

echo "== publish filter: alt-backend consumer leg (dry-run) =="
out=$(run_script --dry-run --publish-only --services alt-backend --role consumer)
assert_contains "$out" "WOULD PUBLISH: consumer=alt-backend" \
  "alt-backend consumer leg publishes its own pacts"
assert_not_contains "$out" "WOULD PUBLISH: consumer=rag-orchestrator" \
  "alt-backend consumer leg must NOT publish rag-orchestrator pacts"
assert_not_contains "$out" "WOULD PUBLISH: consumer=recap-worker" \
  "alt-backend consumer leg must NOT publish recap-worker pacts"

echo "== publish filter: provider leg skips publish entirely (dry-run) =="
out=$(run_script --dry-run --publish-only --services recap-worker --role provider)
assert_not_contains "$out" "WOULD PUBLISH:" \
  "provider leg must not publish any pact files (providers don't generate pacts)"

echo "== publish filter: unfiltered publishes every pact (dry-run) =="
out=$(run_script --dry-run --publish-only)
assert_contains "$out" "WOULD PUBLISH: consumer=alt-backend" \
  "unfiltered: alt-backend pacts are published"
assert_contains "$out" "WOULD PUBLISH: consumer=recap-worker" \
  "unfiltered: recap-worker pacts are published"
assert_contains "$out" "WOULD PUBLISH: consumer=recap-evaluator" \
  "unfiltered: recap-evaluator pacts are published"
assert_contains "$out" "WOULD PUBLISH: consumer=rag-orchestrator" \
  "unfiltered: rag-orchestrator pacts are published"

if [[ $FAILS -gt 0 ]]; then
  echo ""
  echo "FAILED: $FAILS of $TESTS assertions failed"
  exit 1
fi

echo ""
echo "ALL PASSED ($TESTS assertions)"
