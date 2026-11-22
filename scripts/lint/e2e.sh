#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

fail=0
summary=()

run_step() {
  local label="$1"
  shift
  local rc=0
  set +e
  "$@"
  rc=$?
  set -e
  if [ "$rc" -eq 0 ]; then
    summary+=("PASS: $label")
  else
    summary+=("FAIL: $label")
    fail=1
  fi
}

run_step "check-suites" bash e2e/playwright/_lib/check-suites.sh
run_step "e2e-clean-worktree" python3 scripts/tests/test-e2e-clean-worktree.py

printf '%s\n' "${summary[@]}"
exit "$fail"
