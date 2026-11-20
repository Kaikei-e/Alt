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

run_step "bash-n:probe-apparmor-postgres.sh" bash -n scripts/probe-apparmor-postgres.sh
run_step "bash-n:check-apparmor-profile.sh" bash -n scripts/check-apparmor-profile.sh
run_step "bash-n:install-apparmor-profile.sh" bash -n scripts/install-apparmor-profile.sh

printf '%s\n' "${summary[@]}"
exit "$fail"
