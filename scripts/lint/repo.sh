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

run_step "actionlint" go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck=
run_step "workflow-safety-tests" python3 -m unittest discover -s .github/workflows/scripts -p "test_*.py"
run_step "workflow-safety-check" python3 .github/workflows/scripts/workflow-safety-check.py

printf '%s\n' "${summary[@]}"
exit "$fail"
