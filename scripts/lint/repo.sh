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

check_forbidden_triggers_grep() {
  shopt -s nullglob
  local files=(.github/workflows/*.yml .github/workflows/*.yaml)
  shopt -u nullglob
  if [ "${#files[@]}" -eq 0 ]; then
    echo "ERROR: no workflow files found under .github/workflows/" >&2
    return 1
  fi
  local violations=0 f matches rc line ln
  for f in "${files[@]}"; do
    matches=""
    rc=0
    matches="$(grep -nE '^[[:space:]]*(pull_request_target|workflow_run)[[:space:]]*:' "$f")" || rc=$?
    if [ "$rc" -eq 0 ]; then
      while IFS= read -r line; do
        ln="${line%%:*}"
        echo "::error file=$f,line=$ln::Forbidden trigger — 'pull_request_target' and 'workflow_run' run with elevated trust and must not be introduced (ADR-000763 security hardening)." >&2
      done <<< "$matches"
      violations=$((violations + 1))
    elif [ "$rc" -ne 1 ]; then
      echo "ERROR: grep failed on $f (exit $rc)" >&2
      return 1
    fi
  done
  if [ "$violations" -gt 0 ]; then
    echo "FAIL: $violations workflow file(s) use forbidden triggers (grep check)." >&2
    return 1
  fi
  echo "OK: grep check clean."
}

run_step "actionlint" go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck=
run_step "workflow-safety-tests" python3 -m unittest discover -s .github/workflows/scripts -p "test_*.py"
run_step "workflow-forbidden-triggers-grep" check_forbidden_triggers_grep
run_step "workflow-safety-check" python3 .github/workflows/scripts/workflow-safety-check.py
run_step "lint-run-tests" bash scripts/tests/test-lint-run.sh

printf '%s\n' "${summary[@]}"
exit "$fail"
