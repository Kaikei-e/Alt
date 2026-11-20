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

for bad_target in run go python rust deno frontend check-link-vs-url-naming; do
  assert_exit_and_err 2 "invalid lint target" "rejects '$bad_target' explicitly" \
    bash "$RUN_SH" "$bad_target"
done

# LINT_BASE resolution tests for harness.sh and adr.sh
tmp_git="$tmp_repo/git-repo"
tmp_bin="$tmp_repo/stub-bin"
mkdir -p "$tmp_git/scripts/lint" "$tmp_bin"
cp "$REPO_ROOT/scripts/lint/harness.sh" "$tmp_git/scripts/lint/harness.sh"
cp "$REPO_ROOT/scripts/lint/adr.sh" "$tmp_git/scripts/lint/adr.sh"

git -C "$tmp_git" init -b main >/dev/null
git -C "$tmp_git" config user.name "Test"
git -C "$tmp_git" config user.email "test@example.com"

printf 'one\n' >"$tmp_git/a.txt"
git -C "$tmp_git" add a.txt
git -C "$tmp_git" commit -m "c1" >/dev/null
c1="$(git -C "$tmp_git" rev-parse HEAD)"

printf 'two\n' >"$tmp_git/a.txt"
git -C "$tmp_git" add a.txt
git -C "$tmp_git" commit -m "c2" >/dev/null
c2="$(git -C "$tmp_git" rev-parse HEAD)"

printf 'three\n' >"$tmp_git/a.txt"
git -C "$tmp_git" add a.txt
git -C "$tmp_git" commit -m "c3" >/dev/null
c3="$(git -C "$tmp_git" rev-parse HEAD)"

git -C "$tmp_git" update-ref refs/remotes/origin/main "$c3"

cat <<'EOF' >"$tmp_bin/python3"
#!/usr/bin/env bash
echo "stub-python3:$*"
EOF
cat <<'EOF' >"$tmp_bin/docdag"
#!/usr/bin/env bash
echo "stub-docdag:$*"
EOF
cat <<'EOF' >"$tmp_bin/go"
#!/usr/bin/env bash
if [ "${1:-}" = "version" ] && [ "${2:-}" = "-m" ]; then
  printf '%s: go1.27.4\n\tpath\tgithub.com/Kaikei-e/DocDag/cmd/docdag\n\tmod\tgithub.com/Kaikei-e/DocDag\tv0.6.0\th1:stub\n' "${3:-docdag}"
  exit 0
fi
exit 1
EOF
chmod +x "$tmp_bin/python3" "$tmp_bin/docdag" "$tmp_bin/go"

assert_exit_and_err 0 "base=${c2}" "harness.sh unset LINT_BASE falls back to HEAD~1 when HEAD==origin/main" \
  env -u LINT_BASE PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"
assert_exit_and_err 0 "base=${c2}" "harness.sh all-zero LINT_BASE treated as unset" \
  env LINT_BASE="0000000000000000000000000000000000000000" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"
assert_exit_and_err 0 "base=${c1}" "harness.sh valid LINT_BASE overrides computed base" \
  env LINT_BASE="$c1" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"
assert_exit_and_err 1 "cannot be resolved" "harness.sh unresolvable LINT_BASE exits non-zero" \
  env LINT_BASE="does-not-exist-rev" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"

assert_exit_and_err 0 "base=${c2}" "adr.sh unset LINT_BASE falls back to HEAD~1 when HEAD==origin/main" \
  env -u LINT_BASE PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/adr.sh"
assert_exit_and_err 0 "base=${c2}" "adr.sh all-zero LINT_BASE treated as unset" \
  env LINT_BASE="0000000000000000000000000000000000000000" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/adr.sh"
assert_exit_and_err 0 "base=${c1}" "adr.sh valid LINT_BASE overrides computed base" \
  env LINT_BASE="$c1" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/adr.sh"
assert_exit_and_err 1 "cannot be resolved" "adr.sh unresolvable LINT_BASE exits non-zero" \
  env LINT_BASE="does-not-exist-rev" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/adr.sh"

# Diverged PR branch: base branch advanced to c3 after branch forked at c2
git -C "$tmp_git" checkout -b pr-branch "$c2" >/dev/null 2>&1
printf 'pr-one\n' >"$tmp_git/pr.txt"
git -C "$tmp_git" add pr.txt
git -C "$tmp_git" commit -m "p1" >/dev/null
git -C "$tmp_git" update-ref refs/remotes/origin/release "$c3"

assert_exit_and_err 0 "base=${c2}" "harness.sh LINT_BASE=origin/release resolves merge-base fork point c2 when base advanced to c3" \
  env LINT_BASE="origin/release" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"
assert_exit_and_err 0 "base=${c2}" "adr.sh LINT_BASE=origin/release resolves merge-base fork point c2 when base advanced to c3" \
  env LINT_BASE="origin/release" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/adr.sh"

# Unrelated history: orphan commit has no merge-base with HEAD
git -C "$tmp_git" checkout --orphan orphan-branch >/dev/null 2>&1
git -C "$tmp_git" rm -rf . >/dev/null 2>&1
printf 'orphan\n' >"$tmp_git/orphan.txt"
git -C "$tmp_git" add orphan.txt
git -C "$tmp_git" commit -m "orphan" >/dev/null
orphan_sha="$(git -C "$tmp_git" rev-parse HEAD)"
git -C "$tmp_git" checkout pr-branch >/dev/null 2>&1

assert_exit_and_err 1 "cannot compute merge-base" "harness.sh unrelated LINT_BASE exits non-zero" \
  env LINT_BASE="$orphan_sha" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"
assert_exit_and_err 1 "cannot compute merge-base" "adr.sh unrelated LINT_BASE exits non-zero" \
  env LINT_BASE="$orphan_sha" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/adr.sh"

exit "$FAILS"
