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

run_step "regen-service-registry" python3 scripts/regen-service-registry.py --check
run_step "compose-image-audit" python3 scripts/compose-image-audit.py

printf '%s\n' "${summary[@]}"
exit "$fail"
