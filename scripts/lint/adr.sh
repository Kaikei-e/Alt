#!/usr/bin/env bash
# Optional input: LINT_BASE — git revision overriding the auto-detected base
# commit (empty or all-zero SHA is treated as unset).
set -euo pipefail

readonly DOCDAG_VERSION="v0.6.0"

if [ "$#" -gt 1 ] || { [ "$#" -eq 1 ] && [ "$1" != "adr" ]; }; then
  echo "usage: $0 [adr]" >&2
  exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"
cd -- "$repo_root"

for cmd in git docdag go python3; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found in PATH: $cmd" >&2
    exit 1
  fi
done

docdag_bin="$(command -v docdag)"
if ! docdag_mod_info="$(go version -m "$docdag_bin" 2>&1)"; then
  echo "error: failed to inspect docdag version via 'go version -m ${docdag_bin}': ${docdag_mod_info}" >&2
  exit 1
fi
actual_docdag_version="$(awk '$1 == "mod" && $2 == "github.com/Kaikei-e/DocDag" { print $3; exit }' <<<"$docdag_mod_info")"
if [ "$actual_docdag_version" != "$DOCDAG_VERSION" ]; then
  echo "error: docdag version mismatch: expected ${DOCDAG_VERSION}, got '${actual_docdag_version:-unknown}' (${docdag_bin})" >&2
  exit 1
fi

resolve_base() {
  local head_sha raw_base target_sha merge_base
  if ! head_sha="$(git rev-parse --verify --quiet 'HEAD^{commit}')"; then
    echo "error: cannot resolve HEAD commit" >&2
    return 1
  fi

  raw_base="${LINT_BASE:-}"
  if [ -n "$raw_base" ] && [[ ! "$raw_base" =~ ^0+$ ]]; then
    if ! target_sha="$(git rev-parse --verify --quiet "${raw_base}^{commit}")"; then
      echo "error: LINT_BASE '${raw_base}' cannot be resolved to a commit" >&2
      return 1
    fi
    if ! merge_base="$(git merge-base "$target_sha" "$head_sha" 2>/dev/null)" || [ -z "$merge_base" ]; then
      echo "error: cannot compute merge-base between LINT_BASE '${raw_base}' (${target_sha}) and HEAD (${head_sha}) (shallow clone or unrelated history)" >&2
      return 1
    fi
    printf '%s\n' "$merge_base"
    return 0
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
echo "==> [adr] LINT_BASE=${LINT_BASE:-<unset>} base=${base}"

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
