#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
target="${1:-proto}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

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

check_allowlist_drift() {
  local out_dir="$tmp_dir/protovis-out"
  mkdir -p "$out_dir"

  (
    cd "$repo_root/$target"
    buf build --exclude-source-info -o "$tmp_dir/descriptor.binpb"
    cd tools/protovis
    go run . -desc "$tmp_dir/descriptor.binpb" -root "$out_dir"
  )

  local stale=0
  local rel=""
  for rel in \
    "alt-butterfly-facade/internal/server/allowlist_gen.go" \
    "alt-frontend-sv/src/lib/gen/allowlist.ts"; do
    if [ ! -f "$repo_root/$rel" ]; then
      echo "Generated allowlist missing: $rel" >&2
      stale=1
    elif ! diff -u "$repo_root/$rel" "$out_dir/$rel"; then
      stale=1
    fi
  done

  if [ "$stale" -ne 0 ]; then
    echo "Generated allowlists are stale. Re-run protovis and commit the result." >&2
    return 1
  fi
}

run_step "buf-lint" env -C "$repo_root/$target" buf lint
run_step "buf-breaking" env -C "$repo_root/$target" buf breaking --against 'https://github.com/Kaikei-e/Alt.git#branch=main,subdir=proto'
run_step "allowlist-drift" check_allowlist_drift

printf '%s\n' "${summary[@]}"
exit "$fail"
