#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -gt 1 ] || { [ "$#" -eq 1 ] && [ "$1" != "adr" ]; }; then
  echo "usage: $0 [adr]" >&2
  exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"
cd -- "$repo_root"

for cmd in git docdag python3; do
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
status_validate="pass"
status_supersede="pass"
status_lint="pass"

echo "==> [adr] docdag validate --immutable-since ${base}"
if ! docdag validate --immutable-since "$base"; then
  status_validate="fail"
  rc=1
fi

echo "==> [adr] adr_supersede_gate.py --base ${base}"
if ! python3 scripts/agent-harness/adr_supersede_gate.py --base "$base"; then
  status_supersede="fail"
  rc=1
fi

echo "==> [adr] docdag lint --strict"
if ! docdag lint --strict; then
  status_lint="fail"
  rc=1
fi

echo "==> [adr] summary (base=${base})"
printf '  %-24s %s\n' "docdag validate" "$status_validate"
printf '  %-24s %s\n' "adr_supersede_gate" "$status_supersede"
printf '  %-24s %s\n' "docdag lint --strict" "$status_lint"

exit "$rc"
