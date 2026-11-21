#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: $0 <project-dir>" >&2
  exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"
project_rel="${1#./}"
project_rel="${project_rel%/}"

case "$project_rel" in
  ""|/*|..|../*|*/..|*/../*)
    echo "error: invalid project directory: $1" >&2
    exit 2
    ;;
esac

project_dir="${repo_root}/${project_rel}"
if [ ! -d "$project_dir" ] || [ ! -f "${project_dir}/Cargo.toml" ]; then
  echo "error: ${project_rel} is not a Rust project directory (missing Cargo.toml)" >&2
  exit 2
fi

find_toolchain_file() {
  local dir="$1"
  while true; do
    if [ -f "${dir}/rust-toolchain.toml" ]; then
      printf '%s\n' "${dir}/rust-toolchain.toml"
      return 0
    fi
    if [ "$dir" = "$repo_root" ] || [ "$dir" = "/" ]; then
      return 1
    fi
    dir="$(dirname -- "$dir")"
  done
}

if ! toolchain_file="$(find_toolchain_file "$project_dir")"; then
  echo "error: rust-toolchain.toml not found in ${project_rel} (or ancestors up to repo root)" >&2
  exit 1
fi

expected_channel="$(sed -n 's/^[[:space:]]*channel[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$toolchain_file" | head -n 1)"
if [ -z "$expected_channel" ]; then
  echo "error: failed to read channel from ${toolchain_file#"${repo_root}/"}" >&2
  exit 1
fi

declared_targets=()
while IFS= read -r tgt; do
  [ -n "$tgt" ] && declared_targets+=("$tgt")
done < <(sed -n 's/^[[:space:]]*targets[[:space:]]*=[[:space:]]*\[\(.*\)\].*/\1/p' "$toolchain_file" | tr ',' '\n' | sed -n 's/^[[:space:]]*"\([^"]*\)".*/\1/p')

cd -- "$project_dir"

if ! cargo_version_line="$(cargo --version 2>&1)"; then
  echo "error: failed to run 'cargo --version' in ${project_rel} (expected channel '${expected_channel}' from ${toolchain_file#"${repo_root}/"}):" >&2
  echo "$cargo_version_line" >&2
  exit 1
fi

active_version="$(awk '{print $2}' <<<"$cargo_version_line")"
if [ "$active_version" != "$expected_channel" ]; then
  echo "error: active cargo version '${active_version}' does not match channel '${expected_channel}' in ${toolchain_file#"${repo_root}/"} (${cargo_version_line})" >&2
  exit 1
fi

fmt_status="pass"
clippy_status="pass"
rc=0

if ! cargo fmt --all -- --check; then
  fmt_status="fail"
  rc=1
fi

if ! cargo clippy --all-targets --all-features -- -D warnings; then
  clippy_status="fail"
  rc=1
fi

for extra_target in "${declared_targets[@]}"; do
  if ! cargo clippy --lib --target "$extra_target" --all-features -- -D warnings; then
    clippy_status="fail"
    rc=1
  fi
done

printf 'rust-lint summary (%s):\n' "$project_rel"
printf '  fmt: %s\n' "$fmt_status"
printf '  clippy: %s\n' "$clippy_status"

exit "$rc"
