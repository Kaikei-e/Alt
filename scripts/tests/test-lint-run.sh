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
assert_exit_and_err 0 "stub-python3:scripts/tests/test-corpus.py" "harness.sh runs test-corpus.py" \
  env -u LINT_BASE PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"
assert_exit_and_err 0 "stub-python3:scripts/lint/check_generated_code.py --base ${c2}" \
  "harness.sh passes --base to check_generated_code.py" \
  env -u LINT_BASE PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"
assert_exit_and_err 0 "stub-python3:scripts/lint/entrypoint_canary.py" \
  "harness.sh invokes entrypoint_canary.py" \
  env -u LINT_BASE PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"
assert_exit_and_err 0 "base=${c2}" \
  "harness.sh force-push unreachable 40-hex LINT_BASE falls back to HEAD~1" \
  env LINT_BASE="1111111111111111111111111111111111111111" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"
assert_exit_and_err 0 "base=${c2}" \
  "harness.sh LINT_BASE equal to HEAD falls back to HEAD~1" \
  env LINT_BASE="$c3" PATH="$tmp_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"

# Verify harness.sh surfaces CORPUS CAPTURE RATE in summary and fails when test-corpus.py fails
tmp_corpus_bin="$tmp_repo/stub-corpus-bin"
mkdir -p "$tmp_corpus_bin"
cat <<'EOF' >"$tmp_corpus_bin/python3"
#!/usr/bin/env bash
if [ "${1:-}" = "scripts/tests/test-corpus.py" ]; then
  echo "CORPUS CAPTURE RATE: 3/22 (13.6%) [caught=3: C-09,C-21,C-22 | expected-miss=19 | failures=0]"
  exit "${STUB_CORPUS_RC:-0}"
fi
echo "stub-python3:$*"
EOF
chmod +x "$tmp_corpus_bin/python3"

assert_exit_and_err 0 "CORPUS CAPTURE RATE: 3/22 (13.6%)" "harness.sh surfaces CORPUS CAPTURE RATE in summary" \
  env -u LINT_BASE PATH="$tmp_corpus_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"
assert_exit_and_err 1 "corpus                           fail" "harness.sh exits 1 when test-corpus.py fails" \
  env -u LINT_BASE STUB_CORPUS_RC=1 PATH="$tmp_corpus_bin:$PATH" bash "$tmp_git/scripts/lint/harness.sh"
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

# --list and all tests
assert_exit_and_err 2 "usage:" "--list with extra argument exits 2" bash "$RUN_SH" --list extra
assert_exit_and_err 2 "usage:" "all with extra argument exits 2" bash "$RUN_SH" all extra

assert_exact_stdout() {
  local expected_rc="$1" expected_out="$2" desc="$3"
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
  if [ "$out" != "$expected_out" ]; then
    echo "FAIL: $desc"
    echo "  expected:"
    printf '    %s\n' $expected_out
    echo "  got:"
    printf '    %s\n' $out
    FAILS=$((FAILS + 1))
    return
  fi
  echo "PASS: $desc"
}

# 1. --list on a temporary non-git repository
tmp_list_repo="$tmp_repo/list-repo"
mkdir -p "$tmp_list_repo/scripts/lint"
cp "$RUN_SH" "$tmp_list_repo/scripts/lint/run.sh"

for runner in go rust python deno frontend check-link-vs-url-naming; do
  printf '#!/usr/bin/env bash\necho "ran:%s:$1"\n' "$runner" >"$tmp_list_repo/scripts/lint/$runner.sh"
done
for named in repo e2e proto harness; do
  printf '#!/usr/bin/env bash\necho "ran:%s:$1"\n' "$named" >"$tmp_list_repo/scripts/lint/$named.sh"
done

mkdir -p \
  "$tmp_list_repo/svc-go" \
  "$tmp_list_repo/nested/svc-rust" \
  "$tmp_list_repo/svc-py" \
  "$tmp_list_repo/svc-deno" \
  "$tmp_list_repo/svc-fe" \
  "$tmp_list_repo/proto" \
  "$tmp_list_repo/tools/protovis/gen" \
  "$tmp_list_repo/stopped-using-k8s/legacy" \
  "$tmp_list_repo/scripts/harness/corpus/C-01" \
  "$tmp_list_repo/alt-butterfly-facade/internal/gen/proto/v1" \
  "$tmp_list_repo/rag-orchestrator/internal/gen/proto/v1" \
  "$tmp_list_repo/other/gen/stub/v1" \
  "$tmp_list_repo/npm-only" \
  "$tmp_list_repo/shadow"

: >"$tmp_list_repo/svc-go/go.mod"
: >"$tmp_list_repo/svc-go/main.go"
: >"$tmp_list_repo/nested/svc-rust/Cargo.toml"
: >"$tmp_list_repo/svc-py/pyproject.toml"
: >"$tmp_list_repo/svc-deno/deno.json"
: >"$tmp_list_repo/svc-fe/bun.lock"
: >"$tmp_list_repo/proto/buf.yaml"
: >"$tmp_list_repo/tools/protovis/go.mod"
: >"$tmp_list_repo/tools/protovis/main.go"
: >"$tmp_list_repo/tools/protovis/gen/foo.pb.go"
: >"$tmp_list_repo/stopped-using-k8s/legacy/go.mod"
: >"$tmp_list_repo/stopped-using-k8s/legacy/main.go"
: >"$tmp_list_repo/scripts/harness/corpus/C-01/go.mod"
: >"$tmp_list_repo/scripts/harness/corpus/C-01/before.go"
: >"$tmp_list_repo/alt-butterfly-facade/internal/gen/proto/go.mod"
: >"$tmp_list_repo/alt-butterfly-facade/internal/gen/proto/v1/a.pb.go"
: >"$tmp_list_repo/rag-orchestrator/internal/gen/proto/go.mod"
: >"$tmp_list_repo/rag-orchestrator/internal/gen/proto/v1/a.connect.go"
: >"$tmp_list_repo/other/gen/stub/go.mod"
: >"$tmp_list_repo/other/gen/stub/v1/x.pb.go"
: >"$tmp_list_repo/other/gen/stub/v1/x.connect.go"
: >"$tmp_list_repo/npm-only/package.json"
: >"$tmp_list_repo/shadow/go.mod"
printf '#!/usr/bin/env bash\necho "ran:shadow"\n' >"$tmp_list_repo/scripts/lint/shadow.sh"

expected_list="$(printf '%s\n' \
  e2e \
  harness \
  nested/svc-rust \
  proto \
  repo \
  svc-deno \
  svc-fe \
  svc-go \
  svc-py \
  tools/protovis)"

assert_exact_stdout 0 "$expected_list" "--list enumerates sorted unique valid targets in non-git temp repo" \
  env -C /tmp bash "$tmp_list_repo/scripts/lint/run.sh" --list

# 2. --list on a temporary git repository (respects .gitignore and exclusions)
tmp_list_git="$tmp_repo/list-git"
mkdir -p "$tmp_list_git/scripts/lint" "$tmp_list_git/svc-go" "$tmp_list_git/proto" \
  "$tmp_list_git/node_modules/pkg" "$tmp_list_git/stopped-using-k8s/old" \
  "$tmp_list_git/scripts/harness/corpus/C-05" \
  "$tmp_list_git/alt-butterfly-facade/internal/gen/proto/v1"
cp "$RUN_SH" "$tmp_list_git/scripts/lint/run.sh"
printf '#!/usr/bin/env bash\necho "ran:go:$1"\n' >"$tmp_list_git/scripts/lint/go.sh"
printf '#!/usr/bin/env bash\necho "ran:proto:$1"\n' >"$tmp_list_git/scripts/lint/proto.sh"
printf '#!/usr/bin/env bash\necho "ran:repo:$1"\n' >"$tmp_list_git/scripts/lint/repo.sh"
printf 'node_modules/\n' >"$tmp_list_git/.gitignore"
: >"$tmp_list_git/svc-go/go.mod"
: >"$tmp_list_git/svc-go/main.go"
: >"$tmp_list_git/proto/buf.yaml"
: >"$tmp_list_git/node_modules/pkg/go.mod"
: >"$tmp_list_git/stopped-using-k8s/old/go.mod"
: >"$tmp_list_git/stopped-using-k8s/old/main.go"
: >"$tmp_list_git/scripts/harness/corpus/C-05/pyproject.toml"
: >"$tmp_list_git/alt-butterfly-facade/internal/gen/proto/go.mod"
: >"$tmp_list_git/alt-butterfly-facade/internal/gen/proto/v1/a.pb.go"

git -C "$tmp_list_git" init -b main >/dev/null
git -C "$tmp_list_git" config user.name "Test"
git -C "$tmp_list_git" config user.email "test@example.com"
git -C "$tmp_list_git" add -A
git -C "$tmp_list_git" commit -m "init" >/dev/null

expected_git_list="$(printf '%s\n' proto repo svc-go)"
assert_exact_stdout 0 "$expected_git_list" "--list enumerates sorted unique valid targets in git temp repo" \
  env -C /tmp bash "$tmp_list_git/scripts/lint/run.sh" --list

# 3. --list on the real repository
real_list_rc=0
set +e
real_list="$(bash "$RUN_SH" --list 2>&1)"
real_list_rc=$?
set -e
if [ "$real_list_rc" -ne 0 ]; then
  echo "FAIL: real repo --list exits 0 (got rc=$real_list_rc, out=$real_list)"
  FAILS=$((FAILS + 1))
else
  sorted_real_list="$(printf '%s\n' "$real_list" | LC_ALL=C sort -u)"
  if [ "$real_list" != "$sorted_real_list" ]; then
    echo "FAIL: real repo --list output is not sorted and deduplicated"
    FAILS=$((FAILS + 1))
  else
    echo "PASS: real repo --list is sorted and deduplicated"
  fi

  for must_have in \
    alt-backend/app \
    alt-perf \
    alt-perf/k6/mock-rss-server \
    plecto \
    proto \
    proto/tools/protovis \
    adr apparmor compose e2e harness observability registry repo; do
    if ! grep -qxF -- "$must_have" <<<"$real_list"; then
      echo "FAIL: real repo --list missing expected target '$must_have'"
      FAILS=$((FAILS + 1))
    fi
  done

  proto_count="$(grep -cxF -- "proto" <<<"$real_list" || true)"
  if [ "$proto_count" -ne 1 ]; then
    echo "FAIL: real repo --list expected proto once, got $proto_count"
    FAILS=$((FAILS + 1))
  else
    echo "PASS: real repo --list includes proto exactly once"
  fi

  for must_not_have in \
    alt-butterfly-facade/internal/gen/proto \
    rag-orchestrator/internal/gen/proto \
    run all go python rust deno frontend check-link-vs-url-naming; do
    if grep -qxF -- "$must_not_have" <<<"$real_list"; then
      echo "FAIL: real repo --list must not contain '$must_not_have'"
      FAILS=$((FAILS + 1))
    fi
  done
  if grep -q '^stopped-using-k8s' <<<"$real_list" || grep -q '^scripts/harness/corpus' <<<"$real_list"; then
    echo "FAIL: real repo --list must exclude stopped-using-k8s/** and scripts/harness/corpus/**"
    FAILS=$((FAILS + 1))
  else
    echo "PASS: real repo --list excludes stopped-using-k8s, scripts/harness/corpus, generated stubs, and reserved names"
  fi
fi

# 4. all subcommand (all pass and partial fail with summary table)
tmp_all_repo="$tmp_repo/all-repo"
mkdir -p "$tmp_all_repo/scripts/lint" "$tmp_all_repo/a-go" "$tmp_all_repo/m-py" "$tmp_all_repo/proto" "$tmp_all_repo/z-rust"
cp "$RUN_SH" "$tmp_all_repo/scripts/lint/run.sh"
: >"$tmp_all_repo/a-go/go.mod"
: >"$tmp_all_repo/a-go/main.go"
: >"$tmp_all_repo/m-py/pyproject.toml"
: >"$tmp_all_repo/proto/buf.yaml"
: >"$tmp_all_repo/z-rust/Cargo.toml"

printf '#!/usr/bin/env bash\necho "ran:go:$1"\n' >"$tmp_all_repo/scripts/lint/go.sh"
printf '#!/usr/bin/env bash\necho "ran:python:$1"\n' >"$tmp_all_repo/scripts/lint/python.sh"
printf '#!/usr/bin/env bash\necho "ran:rust:$1"\n' >"$tmp_all_repo/scripts/lint/rust.sh"
printf '#!/usr/bin/env bash\necho "ran:proto:$1"\n' >"$tmp_all_repo/scripts/lint/proto.sh"
printf '#!/usr/bin/env bash\necho "ran:repo:$1"\n' >"$tmp_all_repo/scripts/lint/repo.sh"

all_pass_rc=0
set +e
all_pass_out="$(env -C /tmp bash "$tmp_all_repo/scripts/lint/run.sh" all 2>&1)"
all_pass_rc=$?
set -e
if [ "$all_pass_rc" -ne 0 ]; then
  echo "FAIL: all exits 0 when every target passes (got rc=$all_pass_rc, out=$all_pass_out)"
  FAILS=$((FAILS + 1))
else
  all_pass_ok=1
  for expect_pat in \
    "ran:go:a-go" \
    "ran:python:m-py" \
    "ran:proto:proto" \
    "ran:repo:repo" \
    "ran:rust:z-rust" \
    "PASS.*a-go" \
    "PASS.*m-py" \
    "PASS.*proto" \
    "PASS.*repo" \
    "PASS.*z-rust"; do
    if ! grep -qE -- "$expect_pat" <<<"$all_pass_out"; then
      echo "FAIL: all pass output missing pattern '$expect_pat'"
      all_pass_ok=0
      FAILS=$((FAILS + 1))
    fi
  done
  if [ "$all_pass_ok" -eq 1 ]; then
    echo "PASS: all runs every target and prints PASS summary table"
  fi
fi

# Partial failure: m-py fails, z-rust after it still runs, exit 1, summary shows PASS and FAIL
printf '#!/usr/bin/env bash\necho "ran:python:$1"\nexit 1\n' >"$tmp_all_repo/scripts/lint/python.sh"
all_fail_rc=0
set +e
all_fail_out="$(bash "$tmp_all_repo/scripts/lint/run.sh" all 2>&1)"
all_fail_rc=$?
set -e
if [ "$all_fail_rc" -ne 1 ]; then
  echo "FAIL: all exits 1 when a target fails (got rc=$all_fail_rc, out=$all_fail_out)"
  FAILS=$((FAILS + 1))
else
  all_fail_ok=1
  for expect_pat in \
    "ran:go:a-go" \
    "ran:python:m-py" \
    "ran:rust:z-rust" \
    "PASS.*a-go" \
    "FAIL.*m-py" \
    "PASS.*proto" \
    "PASS.*repo" \
    "PASS.*z-rust"; do
    if ! grep -qE -- "$expect_pat" <<<"$all_fail_out"; then
      echo "FAIL: all fail output missing pattern '$expect_pat'"
      all_fail_ok=0
      FAILS=$((FAILS + 1))
    fi
  done
  if [ "$all_fail_ok" -eq 1 ]; then
    echo "PASS: all continues after failure, prints PASS/FAIL summary table, and exits 1"
  fi
fi

# rust.sh extra target (e.g. wasm32-unknown-unknown) verification
tmp_rust_repo="$tmp_repo/rust-target-repo"
tmp_cargo_bin="$tmp_repo/stub-cargo-bin"
mkdir -p "$tmp_rust_repo/scripts/lint" "$tmp_rust_repo/wasm-crate" "$tmp_cargo_bin"
cp "$REPO_ROOT/scripts/lint/rust.sh" "$tmp_rust_repo/scripts/lint/rust.sh"
printf '[package]\nname = "wasm-crate"\nversion = "0.1.0"\n' >"$tmp_rust_repo/wasm-crate/Cargo.toml"
printf '[toolchain]\nchannel = "1.99.1"\ntargets = ["wasm32-unknown-unknown"]\n' >"$tmp_rust_repo/wasm-crate/rust-toolchain.toml"

cat <<'EOF' >"$tmp_cargo_bin/cargo"
#!/usr/bin/env bash
if [ "${1:-}" = "--version" ]; then
  echo "cargo 1.99.1 (stub)"
  exit 0
fi
echo "stub-cargo:$*"
if [ "${STUB_FAIL_WASM:-0}" = "1" ] && [[ "$*" == *"--target wasm32-unknown-unknown"* ]]; then
  exit 1
fi
exit 0
EOF
chmod +x "$tmp_cargo_bin/cargo"

assert_exit_and_err 0 "stub-cargo:clippy --lib --target wasm32-unknown-unknown --all-features -- -D warnings" \
  "rust.sh runs clippy for declared wasm32-unknown-unknown target" \
  env PATH="$tmp_cargo_bin:$PATH" bash "$tmp_rust_repo/scripts/lint/rust.sh" wasm-crate

assert_exit_and_err 1 "clippy: fail" \
  "rust.sh fails when declared wasm32-unknown-unknown target clippy fails" \
  env STUB_FAIL_WASM=1 PATH="$tmp_cargo_bin:$PATH" bash "$tmp_rust_repo/scripts/lint/rust.sh" wasm-crate

# 5. Free disk space guard (CHECK-06)
assert_exit_and_err 1 "ディスク空き容量不足" \
  "run.sh exits 1 with Japanese error when available disk space is below threshold" \
  env ALT_LINT_FAKE_FREE_KB=1024 bash "$tmp_repo/scripts/lint/run.sh" repo

assert_exit_and_err 0 "ran:repo:repo:$tmp_repo" \
  "run.sh passes disk check when available disk space meets threshold" \
  env ALT_LINT_FAKE_FREE_KB=20971520 bash "$tmp_repo/scripts/lint/run.sh" repo

assert_exit_and_err 0 "svc-go" \
  "run.sh --list skips disk space check even when disk space is below threshold" \
  env ALT_LINT_FAKE_FREE_KB=1024 bash "$tmp_list_repo/scripts/lint/run.sh" --list

# 6. Policy mode (CHECK-02)
tmp_policy_repo="$tmp_repo/policy-repo"
mkdir -p "$tmp_policy_repo/scripts/lint"
cp "$RUN_SH" "$tmp_policy_repo/scripts/lint/run.sh"
printf '#!/usr/bin/env bash\necho "policy:${LINT_POLICY:-unset}:target:$1"\n' >"$tmp_policy_repo/scripts/lint/repo.sh"

assert_exit_and_err 0 "policy:fix-existing:target:repo" \
  "run.sh defaults LINT_POLICY to fix-existing" \
  env -u LINT_POLICY bash "$tmp_policy_repo/scripts/lint/run.sh" repo

assert_exit_and_err 0 "policy:fix-existing:target:repo" \
  "run.sh --policy fix-existing propagates LINT_POLICY=fix-existing" \
  env -u LINT_POLICY bash "$tmp_policy_repo/scripts/lint/run.sh" --policy fix-existing repo

assert_exit_and_err 0 "policy:block-new:target:repo" \
  "run.sh --policy block-new propagates LINT_POLICY=block-new" \
  env -u LINT_POLICY bash "$tmp_policy_repo/scripts/lint/run.sh" --policy block-new repo

assert_exit_and_err 0 "policy:block-new:target:repo" \
  "run.sh respects LINT_POLICY=block-new environment variable" \
  env LINT_POLICY=block-new bash "$tmp_policy_repo/scripts/lint/run.sh" repo

assert_exit_and_err 0 "policy:block-new:target:repo" \
  "run.sh --policy block-new all propagates LINT_POLICY to every target" \
  env -u LINT_POLICY bash "$tmp_policy_repo/scripts/lint/run.sh" --policy block-new all

assert_exit_and_err 2 "invalid lint policy" \
  "run.sh --policy foo exits 2" \
  bash "$tmp_policy_repo/scripts/lint/run.sh" --policy foo repo

assert_exit_and_err 2 "invalid lint policy" \
  "run.sh rejects invalid LINT_POLICY env value with exit 2" \
  env LINT_POLICY=foo bash "$tmp_policy_repo/scripts/lint/run.sh" repo

assert_exit_and_err 2 "usage:" \
  "run.sh --policy without target exits 2" \
  bash "$tmp_policy_repo/scripts/lint/run.sh" --policy block-new

# 7. Dirty worktree resilience (CHECK-06)
tmp_dirty_git="$tmp_repo/dirty-git"
tmp_proto_bin="$tmp_repo/stub-proto-bin"
mkdir -p \
  "$tmp_dirty_git/scripts/lint" \
  "$tmp_dirty_git/svc-go" \
  "$tmp_dirty_git/proto/tools/protovis" \
  "$tmp_dirty_git/alt-butterfly-facade/internal/server" \
  "$tmp_dirty_git/alt-frontend-sv/src/lib/gen" \
  "$tmp_proto_bin"
cp "$RUN_SH" "$tmp_dirty_git/scripts/lint/run.sh"
cp "$REPO_ROOT/scripts/lint/proto.sh" "$tmp_dirty_git/scripts/lint/proto.sh"
cp "$REPO_ROOT/scripts/lint/canary_audit.py" "$tmp_dirty_git/scripts/lint/canary_audit.py"
cp "$REPO_ROOT/alt-frontend-sv/biome.json" "$tmp_dirty_git/alt-frontend-sv/biome.json"
printf 'alt-frontend-sv/test-results/\n' >"$tmp_dirty_git/.gitignore"
printf 'module svc-go\n' >"$tmp_dirty_git/svc-go/go.mod"
printf 'package main\n' >"$tmp_dirty_git/svc-go/main.go"
printf 'version: v2\n' >"$tmp_dirty_git/proto/buf.yaml"
printf '// allowlist\n' >"$tmp_dirty_git/alt-butterfly-facade/internal/server/allowlist_gen.go"
printf '// allowlist\n' >"$tmp_dirty_git/alt-frontend-sv/src/lib/gen/allowlist.ts"
printf 'package main\n' >"$tmp_dirty_git/proto/tools/protovis/main.go"

git -C "$tmp_dirty_git" init -b main >/dev/null
git -C "$tmp_dirty_git" config user.name "Test"
git -C "$tmp_dirty_git" config user.email "test@example.com"
git -C "$tmp_dirty_git" add -A
git -C "$tmp_dirty_git" commit -m "init dirty fixture" >/dev/null

clean_dirty_list="$(bash "$tmp_dirty_git/scripts/lint/run.sh" --list)"

# Dirty the worktree with untracked test-results artifact, uncommitted tracked change, and unrelated untracked file
mkdir -p "$tmp_dirty_git/alt-frontend-sv/test-results"
printf '{"status":"ok"}\n' >"$tmp_dirty_git/alt-frontend-sv/test-results/artifact.json"
printf 'package main\n// dirty edit\n' >"$tmp_dirty_git/svc-go/main.go"
printf 'unrelated scratch\n' >"$tmp_dirty_git/unrelated-dirty.txt"

assert_exact_stdout 0 "$clean_dirty_list" \
  "run.sh --list is unaffected by dirty worktree and untracked test-results artifacts" \
  bash "$tmp_dirty_git/scripts/lint/run.sh" --list

# Verify Biome VCS .gitignore config ignores test-results/ in real biome.json and real repo .gitignore
assert_exit_and_err 0 "biome-vcs-ok" \
  "alt-frontend-sv/biome.json enables vcs useIgnoreFile and excludes test-results" \
  python3 -c '
import json, pathlib, sys
cfg = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
assert cfg["vcs"]["enabled"] is True
assert cfg["vcs"]["clientKind"] == "git"
assert cfg["vcs"]["useIgnoreFile"] is True
assert "!test-results" in cfg["files"]["includes"]
print("biome-vcs-ok")
' "$REPO_ROOT/alt-frontend-sv/biome.json"

assert_exit_and_err 0 "alt-frontend-sv/test-results/artifact.json" \
  "real repo .gitignore ignores alt-frontend-sv/test-results/artifact.json" \
  git -C "$REPO_ROOT" check-ignore --no-index "alt-frontend-sv/test-results/artifact.json"

# Stub buf & go to verify proto.sh succeeds even when unrelated uncommitted files exist in worktree
cat <<'EOF' >"$tmp_proto_bin/buf"
#!/usr/bin/env bash
case "${1:-}" in
  --version)
    echo "1.48.0"
    ;;
  build)
    out=""
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-o" ]; then
        out="$2"
        shift 2
        continue
      fi
      shift
    done
    [ -n "$out" ] && : >"$out"
    ;;
  lint)
    if [ -f canary.proto ]; then
      echo 'canary.proto:5:6:Enum name "bad_enum" should be PascalCase.' >&2
      exit 1
    fi
    exit 0
    ;;
  breaking)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
EOF
cat <<'EOF' >"$tmp_proto_bin/go"
#!/usr/bin/env bash
out_root=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-root" ]; then
    out_root="$2"
    shift 2
    continue
  fi
  shift
done
if [ -n "$out_root" ]; then
  mkdir -p "$out_root/alt-butterfly-facade/internal/server" "$out_root/alt-frontend-sv/src/lib/gen"
  printf '// allowlist\n' >"$out_root/alt-butterfly-facade/internal/server/allowlist_gen.go"
  printf '// allowlist\n' >"$out_root/alt-frontend-sv/src/lib/gen/allowlist.ts"
fi
exit 0
EOF
chmod +x "$tmp_proto_bin/buf" "$tmp_proto_bin/go"

assert_exit_and_err 0 "PASS: allowlist-drift" \
  "proto.sh passes in dirty git worktree with unrelated uncommitted files" \
  env PATH="$tmp_proto_bin:$PATH" bash "$tmp_dirty_git/scripts/lint/proto.sh" proto

exit "$FAILS"
