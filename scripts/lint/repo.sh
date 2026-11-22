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

resolve_ast_grep() {
  if command -v ast-grep >/dev/null 2>&1; then
    command -v ast-grep
    return 0
  fi
  if [ -x "${HOME}/.local/bin/ast-grep" ]; then
    printf '%s\n' "${HOME}/.local/bin/ast-grep"
    return 0
  fi
  echo "ERROR: ast-grep not found in PATH or ${HOME}/.local/bin/ast-grep" >&2
  return 1
}

run_ast_grep_rule_tests() {
  local bin
  bin="$(resolve_ast_grep)" || return 1
  "$bin" test -c scripts/lint/ast-grep/sgconfig.yml --skip-snapshot-tests
}

run_ast_grep_scan() {
  local bin
  bin="$(resolve_ast_grep)" || return 1
  "$bin" scan -c scripts/lint/ast-grep/sgconfig.yml
}

run_step "actionlint" go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck=
run_step "workflow-safety-tests" python3 -m unittest discover -s .github/workflows/scripts -p "test_*.py"
run_step "workflow-forbidden-triggers-grep" check_forbidden_triggers_grep
run_step "workflow-safety-check" python3 .github/workflows/scripts/workflow-safety-check.py
run_step "lint-run-tests" bash scripts/tests/test-lint-run.sh
run_step "lint-canary-tests" python3 scripts/tests/test-lint-canary.py
run_step "suppression-check-tests" python3 scripts/tests/test-check-suppressions.py
run_step "suppression-governance" python3 scripts/lint/check_suppressions.py
run_step "ast-grep-rule-tests" run_ast_grep_rule_tests
run_step "ast-grep-scan" run_ast_grep_scan
run_step "go-canonical-sync" python3 scripts/codegen/sync_go_canonical.py --check
run_step "python-pki-sync" python3 scripts/codegen/pki_py/sync_python_pki.py --check
run_step "python-version-parity" python3 scripts/lint/check_python_versions.py

printf '%s\n' "${summary[@]}"
exit "$fail"
