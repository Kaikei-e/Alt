#!/usr/bin/env bash
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
  local head_sha origin_main_sha merge_base
  if ! head_sha="$(git rev-parse --verify --quiet 'HEAD^{commit}')"; then
    echo "error: cannot resolve HEAD commit" >&2
    return 1
  fi
  if ! origin_main_sha="$(git rev-parse --verify --quiet 'origin/main^{commit}')"; then
    echo "error: cannot resolve origin/main (fetch full history from origin)" >&2
    return 1
  fi
  if ! merge_base="$(git merge-base "$head_sha" "$origin_main_sha" 2>/dev/null)" || [ -z "$merge_base" ]; then
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

rc=0
status_selfcheck="pass"
status_postmortem="pass"
status_migration="pass"

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

echo "==> [harness] summary (base=${base})"
printf '  %-28s %s\n' "harness_selfcheck" "$status_selfcheck"
printf '  %-28s %s\n' "postmortem_ledger" "$status_postmortem"
printf '  %-28s %s\n' "migration_destructive_gate" "$status_migration"

exit "$rc"
