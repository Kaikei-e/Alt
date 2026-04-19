#!/usr/bin/env bash
# Filter-behaviour tests for scripts/pact-check.sh.
#
# The script now supports three additive flags on top of the existing
# --services CSV filter:
#
#   --role consumer|provider   tighten matching to labels of that role
#   --dry-run                  print "WOULD RUN: <label>" per step, no exec
#   --publish-manual-verifications  run ONLY the manual-verification block
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

# --- publish-manual-verifications sub-command runs only the manual block ---
echo "== --publish-manual-verifications (dry-run, default filter) =="
out=$(run_script --dry-run --publish-only --publish-manual-verifications)
assert_contains "$out" "WOULD POST MANUAL VERIFICATION:" \
  "manual-verification subcommand announces the three bridging records"
assert_not_contains "$out" "WOULD RUN: Go: alt-backend consumer" \
  "manual-verification subcommand does not run unit pact steps"
assert_not_contains "$out" "WOULD RUN: Python: news-creator provider" \
  "manual-verification subcommand does not run provider verifications"

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

if [[ $FAILS -gt 0 ]]; then
  echo ""
  echo "FAILED: $FAILS of $TESTS assertions failed"
  exit 1
fi

echo ""
echo "ALL PASSED ($TESTS assertions)"
