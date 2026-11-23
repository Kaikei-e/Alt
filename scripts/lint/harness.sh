#!/usr/bin/env bash
# Optional input: LINT_BASE — git revision overriding the auto-detected base
# commit (empty or all-zero SHA is treated as unset).
set -euo pipefail

if [ "$#" -gt 1 ] || { [ "$#" -eq 1 ] && [ "$1" != "harness" ]; }; then
  echo "usage: $0 [harness]" >&2
  exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"
cd -- "$repo_root"

for cmd in git python3; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found in PATH: $cmd" >&2
    exit 1
  fi
done

resolve_base() {
  local head_sha raw_base target_sha merge_base
  if ! head_sha="$(git rev-parse --verify --quiet 'HEAD^{commit}')"; then
    echo "error: cannot resolve HEAD commit" >&2
    return 1
  fi

  raw_base="${LINT_BASE:-}"
  if [ -n "$raw_base" ] && [[ ! "$raw_base" =~ ^0+$ ]]; then
    if target_sha="$(git rev-parse --verify --quiet "${raw_base}^{commit}")"; then
      if ! merge_base="$(git merge-base "$target_sha" "$head_sha" 2>/dev/null)" || [ -z "$merge_base" ]; then
        echo "error: cannot compute merge-base between LINT_BASE '${raw_base}' (${target_sha}) and HEAD (${head_sha}) (shallow clone or unrelated history)" >&2
        return 1
      fi
      if [ "$merge_base" = "$head_sha" ]; then
        if ! merge_base="$(git rev-parse --verify --quiet 'HEAD~1^{commit}')"; then
          echo "error: LINT_BASE '${raw_base}' matches HEAD and HEAD~1 cannot be resolved (shallow clone?)" >&2
          return 1
        fi
      fi
      printf '%s\n' "$merge_base"
      return 0
    fi
    if [[ ! "$raw_base" =~ ^[0-9a-fA-F]{40}$ ]]; then
      echo "error: LINT_BASE '${raw_base}' cannot be resolved to a commit" >&2
      return 1
    fi
    echo "warn: LINT_BASE '${raw_base}' not found (force-push?); falling back to origin/main / HEAD~1" >&2
  fi

  if ! target_sha="$(git rev-parse --verify --quiet 'origin/main^{commit}')"; then
    echo "error: cannot resolve origin/main (fetch full history from origin)" >&2
    return 1
  fi
  if ! merge_base="$(git merge-base "$head_sha" "$target_sha" 2>/dev/null)" || [ -z "$merge_base" ]; then
    echo "error: cannot compute merge-base between HEAD and origin/main (shallow clone or unrelated history)" >&2
    return 1
  fi
  if [ "$merge_base" = "$head_sha" ]; then
    if ! merge_base="$(git rev-parse --verify --quiet 'HEAD~1^{commit}')"; then
      echo "error: HEAD matches merge-base with origin/main and HEAD~1 cannot be resolved (shallow clone?)" >&2
      return 1
    fi
  fi
  printf '%s\n' "$merge_base"
}

base="$(resolve_base)"
echo "==> [harness] LINT_BASE=${LINT_BASE:-<unset>} base=${base}"

rc=0
status_selfcheck="pass"
status_postmortem="pass"
status_migration="pass"
status_guard_branches="pass"
status_task_packet_templates="pass"
status_task_packet_tests="pass"
status_go_canonical_sync="pass"
status_go_canonical_tests="pass"
status_python_pki_sync="pass"
status_python_pki_tests="pass"
status_python_version_tests="pass"
status_pytest_randomized_tests="pass"
status_behavior_screener_tests="pass"
status_suppression_ratchet="pass"
status_suppression_ratchet_tests="pass"
status_suppression_diff_report="pass"
status_suppression_diff_tests="pass"
status_generated_code="pass"
status_generated_code_tests="pass"
status_worktree_hygiene="pass"
status_worktree_hygiene_tests="pass"
status_canary_ledger="pass"
status_entrypoint_canary="pass"
status_exec_guards_and_runs="skip"
status_corpus="pass"
corpus_capture_rate=""

echo "==> [harness] harness_selfcheck.py"
if ! python3 scripts/agent-harness/harness_selfcheck.py; then
  status_selfcheck="fail"
  rc=1
fi

echo "==> [harness] postmortem_ledger.py --check --base ${base}"
if ! python3 scripts/agent-harness/postmortem_ledger.py --check --base "$base"; then
  status_postmortem="fail"
  rc=1
fi

echo "==> [harness] migration_destructive_gate.py --base ${base}"
if ! python3 scripts/agent-harness/migration_destructive_gate.py --base "$base"; then
  status_migration="fail"
  rc=1
fi

echo "==> [harness] test-guard-ops-commands-branches.py"
if ! python3 scripts/tests/test-guard-ops-commands-branches.py; then
  status_guard_branches="fail"
  rc=1
fi

echo "==> [harness] task_packet.py check-templates"
if ! python3 scripts/agent-harness/task_packet.py check-templates; then
  status_task_packet_templates="fail"
  rc=1
fi

echo "==> [harness] test-task-packet.py"
if ! python3 scripts/tests/test-task-packet.py; then
  status_task_packet_tests="fail"
  rc=1
fi

echo "==> [harness] sync_go_canonical.py --check"
if ! python3 scripts/codegen/sync_go_canonical.py --check; then
  status_go_canonical_sync="fail"
  rc=1
fi

echo "==> [harness] test-sync-go-canonical.py"
if ! python3 scripts/tests/test-sync-go-canonical.py; then
  status_go_canonical_tests="fail"
  rc=1
fi

echo "==> [harness] sync_python_pki.py --check"
if ! python3 scripts/codegen/pki_py/sync_python_pki.py --check; then
  status_python_pki_sync="fail"
  rc=1
fi

echo "==> [harness] test-sync-python-pki.py"
if ! python3 scripts/tests/test-sync-python-pki.py; then
  status_python_pki_tests="fail"
  rc=1
fi

echo "==> [harness] test-check-python-versions.py"
if ! python3 scripts/tests/test-check-python-versions.py; then
  status_python_version_tests="fail"
  rc=1
fi

echo "==> [harness] test-pytest-randomized.py"
if ! python3 scripts/tests/test-pytest-randomized.py; then
  status_pytest_randomized_tests="fail"
  rc=1
fi

echo "==> [harness] test-behavior-screener.py"
if ! python3 scripts/tests/test-behavior-screener.py; then
  status_behavior_screener_tests="fail"
  rc=1
fi

echo "==> [harness] check_ratchet.py"
if ! python3 scripts/lint/check_ratchet.py; then
  status_suppression_ratchet="fail"
  rc=1
fi

echo "==> [harness] test-check-ratchet.py"
if ! python3 scripts/tests/test-check-ratchet.py; then
  status_suppression_ratchet_tests="fail"
  rc=1
fi

echo "==> [harness] suppression_diff_report.py --base ${base}"
if ! python3 scripts/lint/suppression_diff_report.py --base "$base"; then
  status_suppression_diff_report="fail"
  rc=1
fi

echo "==> [harness] test-suppression-diff-report.py"
if ! python3 scripts/tests/test-suppression-diff-report.py; then
  status_suppression_diff_tests="fail"
  rc=1
fi

echo "==> [harness] check_generated_code.py --base ${base}"
if ! python3 scripts/lint/check_generated_code.py --base "$base"; then
  status_generated_code="fail"
  rc=1
fi

echo "==> [harness] test-check-generated-code.py"
if ! python3 scripts/tests/test-check-generated-code.py; then
  status_generated_code_tests="fail"
  rc=1
fi

echo "==> [harness] check_worktree_hygiene.py"
if ! python3 scripts/lint/check_worktree_hygiene.py; then
  status_worktree_hygiene="fail"
  rc=1
fi

echo "==> [harness] test-worktree-hygiene.py"
if ! python3 scripts/tests/test-worktree-hygiene.py; then
  status_worktree_hygiene_tests="fail"
  rc=1
fi

echo "==> [harness] canary_audit.py verify-ledger --repo-root ."
if ! python3 scripts/lint/canary_audit.py verify-ledger --repo-root .; then
  status_canary_ledger="fail"
  rc=1
fi

echo "==> [harness] entrypoint_canary.py --languages proto --skip-baseline"
if ! python3 scripts/lint/entrypoint_canary.py --languages proto --skip-baseline; then
  status_entrypoint_canary="fail"
  rc=1
fi

if [ -f scripts/tests/test-exec-guards-and-runs.py ]; then
  echo "==> [harness] test-exec-guards-and-runs.py"
  status_exec_guards_and_runs="pass"
  if ! python3 scripts/tests/test-exec-guards-and-runs.py; then
    status_exec_guards_and_runs="fail"
    rc=1
  fi
fi

echo "==> [harness] test-corpus.py"
corpus_out_file="$(mktemp "${TMPDIR:-/tmp}/alt-harness-corpus-XXXXXX")"
trap 'rm -f "$corpus_out_file"' EXIT
corpus_rc=0
set +e
python3 scripts/tests/test-corpus.py 2>&1 | tee "$corpus_out_file"
corpus_rc="${PIPESTATUS[0]}"
set -e
if [ "$corpus_rc" -ne 0 ]; then
  status_corpus="fail"
  rc=1
fi
corpus_capture_rate="$(grep -E '^CORPUS CAPTURE RATE:' "$corpus_out_file" | tail -n 1 || true)"

echo "==> [harness] summary (base=${base})"
printf '  %-32s %s\n' "harness_selfcheck" "$status_selfcheck"
printf '  %-32s %s\n' "postmortem_ledger" "$status_postmortem"
printf '  %-32s %s\n' "migration_destructive_gate" "$status_migration"
printf '  %-32s %s\n' "guard_ops_commands_branches" "$status_guard_branches"
printf '  %-32s %s\n' "task_packet_templates" "$status_task_packet_templates"
printf '  %-32s %s\n' "task_packet_tests" "$status_task_packet_tests"
printf '  %-32s %s\n' "go_canonical_sync" "$status_go_canonical_sync"
printf '  %-32s %s\n' "go_canonical_tests" "$status_go_canonical_tests"
printf '  %-32s %s\n' "python_pki_sync" "$status_python_pki_sync"
printf '  %-32s %s\n' "python_pki_tests" "$status_python_pki_tests"
printf '  %-32s %s\n' "python_version_tests" "$status_python_version_tests"
printf '  %-32s %s\n' "pytest_randomized_tests" "$status_pytest_randomized_tests"
printf '  %-32s %s\n' "behavior_screener_tests" "$status_behavior_screener_tests"
printf '  %-32s %s\n' "suppression_ratchet" "$status_suppression_ratchet"
printf '  %-32s %s\n' "suppression_ratchet_tests" "$status_suppression_ratchet_tests"
printf '  %-32s %s\n' "suppression_diff_report" "$status_suppression_diff_report"
printf '  %-32s %s\n' "suppression_diff_tests" "$status_suppression_diff_tests"
printf '  %-32s %s\n' "generated_code" "$status_generated_code"
printf '  %-32s %s\n' "generated_code_tests" "$status_generated_code_tests"
printf '  %-32s %s\n' "worktree_hygiene" "$status_worktree_hygiene"
printf '  %-32s %s\n' "worktree_hygiene_tests" "$status_worktree_hygiene_tests"
printf '  %-32s %s\n' "canary_ledger" "$status_canary_ledger"
printf '  %-32s %s\n' "entrypoint_canary" "$status_entrypoint_canary"
printf '  %-32s %s\n' "exec_guards_and_runs_tests" "$status_exec_guards_and_runs"
printf '  %-32s %s\n' "corpus" "$status_corpus"
if [ -n "$corpus_capture_rate" ]; then
  printf '  %s\n' "$corpus_capture_rate"
fi

exit "$rc"
