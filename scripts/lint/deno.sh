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

cd "$project_path"

fmt_status="pass"
if ! "$deno_bin" fmt --check; then
  fmt_status="fail"
fi

lint_status="pass"
if ! "$deno_bin" lint; then
  lint_status="fail"
fi

echo "=== deno.sh summary (${project_dir}, deno ${pinned}) ==="
echo "deno fmt --check: ${fmt_status}"
echo "deno lint: ${lint_status}"

if [[ "$fmt_status" != "pass" || "$lint_status" != "pass" ]]; then
  exit 1
fi
