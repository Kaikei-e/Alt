#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUN_SH="$REPO_ROOT/scripts/lint/run.sh"

FAILS=0

assert_exit_and_err() {
  local expected_rc="$1" expected_substr="$2" desc="$3"
  shift 3
  local out="" rc=0
  set +e
  out="$("$@" 2>&1)"
  rc=$?
  set -e
  if [ "$rc" -ne "$expected_rc" ]; then
    echo "FAIL: $desc (expected rc=$expected_rc, got rc=$rc, out=$out)"
    FAILS=$((FAILS + 1))
    return
  fi
  if [ -n "$expected_substr" ] && ! grep -qF -- "$expected_substr" <<<"$out"; then
    echo "FAIL: $desc (missing $expected_substr in: $out)"
    FAILS=$((FAILS + 1))
    return
  fi
  echo "PASS: $desc"
}

assert_exit_and_err 2 "usage:" "missing argument exits 2" bash "$RUN_SH"
assert_exit_and_err 2 "usage:" "empty argument exits 2" bash "$RUN_SH" ""
assert_exit_and_err 2 "usage:" "extra arguments exit 2" bash "$RUN_SH" repo extra
assert_exit_and_err 2 "not found" "missing directory exits 2" bash "$RUN_SH" does-not-exist-dir
assert_exit_and_err 2 "invalid lint target" "parent traversal exits 2" bash "$RUN_SH" "../outside"
assert_exit_and_err 2 "no recognized lint marker" "directory without marker exits 2" bash "$RUN_SH" docs

tmp_repo="$(mktemp -d)"
trap 'rm -rf "$tmp_repo"' EXIT
mkdir -p "$tmp_repo/scripts/lint"
cp "$RUN_SH" "$tmp_repo/scripts/lint/run.sh"

for spec in \
  "svc-go:go.mod:go" \
  "svc-rust:Cargo.toml:rust" \
  "svc-py:pyproject.toml:python" \
  "svc-deno:deno.json:deno" \
  "svc-fe:bun.lock:frontend" \
  "proto:buf.yaml:proto"; do
  IFS=: read -r dir marker kind <<<"$spec"
  mkdir -p "$tmp_repo/$dir"
  : >"$tmp_repo/$dir/$marker"
  printf '#!/usr/bin/env bash\necho "ran:%s:%s:%s"\n' "$kind" '$1' '$(pwd)' >"$tmp_repo/scripts/lint/$kind.sh"
  assert_exit_and_err 0 "ran:$kind:$dir:$tmp_repo" "dispatches $dir via $marker to $kind.sh" \
    env -C /tmp bash "$tmp_repo/scripts/lint/run.sh" "$dir/"
done

mkdir -p "$tmp_repo/npm-only"
: >"$tmp_repo/npm-only/package.json"
assert_exit_and_err 2 "no recognized lint marker" "package.json without bun.lock is not dispatched as frontend" \
  bash "$tmp_repo/scripts/lint/run.sh" npm-only

printf '#!/usr/bin/env bash\necho "ran:repo:%s:%s"\n' '$1' '$(pwd)' >"$tmp_repo/scripts/lint/repo.sh"
assert_exit_and_err 0 "ran:repo:repo:$tmp_repo" "dispatches named target repo from outside repo root" \
  env -C /tmp bash "$tmp_repo/scripts/lint/run.sh" repo

mkdir -p "$tmp_repo/e2e/playwright"
: >"$tmp_repo/e2e/playwright/package.json"
printf '#!/usr/bin/env bash\necho "ran:e2e:%s:%s"\n' '$1' '$(pwd)' >"$tmp_repo/scripts/lint/e2e.sh"
assert_exit_and_err 0 "ran:e2e:e2e:$tmp_repo" "dispatches named target e2e when directory has no marker" \
  bash "$tmp_repo/scripts/lint/run.sh" e2e

mkdir -p "$tmp_repo/shadow"
: >"$tmp_repo/shadow/go.mod"
printf '#!/usr/bin/env bash\necho "ran:shadow"\n' >"$tmp_repo/scripts/lint/shadow.sh"
assert_exit_and_err 2 "shadows directory shadow/" "named target shadowing marker directory exits 2" \
  bash "$tmp_repo/scripts/lint/run.sh" shadow

exit "$FAILS"
