#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 || -z "${1:-}" ]]; then
  echo "Usage: $0 <project-dir>" >&2
  exit 2
fi

project_dir="${1%/}"
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "${script_dir}/../.." && pwd)"
project_path="${repo_root}/${project_dir}"

if [[ ! -d "$project_path" ]]; then
  echo "Project directory does not exist: ${project_path}" >&2
  exit 1
fi

pin_file="${project_path}/.dvmrc"
if [[ ! -f "$pin_file" ]]; then
  echo "Missing Deno version pin file: ${pin_file}" >&2
  exit 1
fi

pinned="$(tr -d '[:space:]' < "$pin_file")"
pinned="${pinned#v}"
if [[ -z "$pinned" ]]; then
  echo "Empty Deno version in ${pin_file}" >&2
  exit 1
fi

deno_version_of() {
  local bin="$1"
  local out first _ ver
  if ! out="$("$bin" --version 2>/dev/null)"; then
    return 1
  fi
  first="${out%%$'\n'*}"
  read -r _ ver _ <<< "$first"
  printf '%s\n' "${ver:-}"
}

deno_bin=""
if command -v deno >/dev/null 2>&1; then
  path_deno="$(command -v deno)"
  if path_ver="$(deno_version_of "$path_deno")" && [[ "$path_ver" == "$pinned" ]]; then
    deno_bin="$path_deno"
  fi
fi

if [[ -z "$deno_bin" ]]; then
  cache_dir="${XDG_CACHE_HOME:-${HOME}/.cache}/alt-lint/deno/${pinned}"
  cached_deno="${cache_dir}/deno"
  if [[ -x "$cached_deno" ]] && cached_ver="$(deno_version_of "$cached_deno")" && [[ "$cached_ver" == "$pinned" ]]; then
    deno_bin="$cached_deno"
  else
    if ! command -v deno >/dev/null 2>&1; then
      echo "Deno ${pinned} is required (${pin_file}), but no deno binary was found on PATH or at ${cached_deno}" >&2
      exit 1
    fi
    mkdir -p "$cache_dir"
    tmp_deno="${cache_dir}/deno.tmp.$$"
    rm -f "$tmp_deno"
    deno upgrade --force --version "$pinned" --output "$tmp_deno"
    chmod +x "$tmp_deno"
    mv -f "$tmp_deno" "$cached_deno"
    deno_bin="$cached_deno"
  fi
fi

actual_ver=""
if ! actual_ver="$(deno_version_of "$deno_bin")"; then
  actual_ver=""
fi
if [[ "$actual_ver" != "$pinned" ]]; then
  echo "Expected Deno ${pinned} (from ${pin_file}), but resolved ${actual_ver:-unknown} at ${deno_bin}" >&2
  exit 1
fi

canary_audit_script="${script_dir}/canary_audit.py"
if [[ -f "${canary_audit_script}" ]]; then
  echo "==> [${project_dir}] canary audit (deno lint)"
  if ! python3 "${canary_audit_script}" deno-canary \
    --project-dir "${project_path}" \
    --deno-bin "${deno_bin}"; then
    exit 1
  fi
elif [[ -f "${repo_root}/scripts/lint/run.sh" ]]; then
  echo "Error: missing canary audit helper: ${canary_audit_script}" >&2
  exit 1
fi

cd "$project_path"

fmt_status="pass"
if ! "$deno_bin" fmt --check; then
  fmt_status="fail"
fi

lint_status="pass"
lint_policy="${LINT_POLICY:-fix-existing}"
if [[ "$lint_policy" == "block-new" ]]; then
  deno_json="$(mktemp "${TMPDIR:-/tmp}/alt-deno-lint-XXXXXX.json")"
  deno_rc=0
  set +e
  "$deno_bin" lint --json > "$deno_json"
  deno_rc=$?
  set -e
  if [[ "$deno_rc" -ne 0 ]]; then
    if python3 "${script_dir}/check_ratchet.py" check-fingerprints \
      --repo-root "${repo_root}" \
      --project-dir "${project_path}" \
      --deno-json "$deno_json" \
      --require-findings; then
      lint_status="pass (block-new)"
    else
      lint_status="fail"
    fi
  fi
  rm -f "$deno_json"
else
  if ! "$deno_bin" lint; then
    lint_status="fail"
  fi
fi

echo "=== deno.sh summary (${project_dir}, deno ${pinned}) ==="
echo "deno fmt --check: ${fmt_status}"
echo "deno lint: ${lint_status}"

if [[ "$fmt_status" != "pass" || "$lint_status" == "fail" ]]; then
  exit 1
fi
