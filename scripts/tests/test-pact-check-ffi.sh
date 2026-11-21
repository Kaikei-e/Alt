#!/usr/bin/env bash
# Hermetic FFI provisioning tests for scripts/pact-check.sh.
#
# Exercises check_ffi and toolchain detection without network or Pact Broker
# by invoking file-based mode (--services altctl --role consumer) with stub
# executables on PATH and an isolated HOME / PACT_EVIDENCE_DIR.

set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")"/.. && pwd)/pact-check.sh"
REPO_ROOT="$(cd "$(dirname "$0")"/../.. && pwd)"
TESTS=0
FAILS=0

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

# Collect minimal POSIX/coreutils commands needed by pact-check.sh so tests
# can control whether `go` and `python3` exist on PATH even on hosts that have
# them installed in /usr/bin or /usr/local/go/bin.
CLEAN_BIN="$TMP_ROOT/clean-bin"
mkdir -p "$CLEAN_BIN"
for tool in bash cat date dirname env grep ls mkdir rm sleep tr; do
  resolved="$(command -v "$tool")"
  ln -s "$resolved" "$CLEAN_BIN/$tool"
done

assert_contains() {
  local output="$1" needle="$2" desc="$3"
  TESTS=$((TESTS + 1))
  if grep -qF -- "$needle" <<<"$output"; then
    echo "  PASS: $desc"
  else
    echo "  FAIL: $desc"
    echo "    expected output to contain: $needle"
    FAILS=$((FAILS + 1))
  fi
}

assert_not_contains() {
  local output="$1" needle="$2" desc="$3"
  TESTS=$((TESTS + 1))
  if grep -qF -- "$needle" <<<"$output"; then
    echo "  FAIL: $desc"
    echo "    expected output NOT to contain: $needle"
    FAILS=$((FAILS + 1))
  else
    echo "  PASS: $desc"
  fi
}

assert_rc_zero() {
  local rc="$1" desc="$2"
  TESTS=$((TESTS + 1))
  if [[ "$rc" -eq 0 ]]; then
    echo "  PASS: $desc"
  else
    echo "  FAIL: $desc"
    echo "    expected exit code 0, got $rc"
    FAILS=$((FAILS + 1))
  fi
}

assert_rc_nonzero() {
  local rc="$1" desc="$2"
  TESTS=$((TESTS + 1))
  if [[ "$rc" -ne 0 ]]; then
    echo "  PASS: $desc"
  else
    echo "  FAIL: $desc"
    echo "    expected non-zero exit code, got 0"
    FAILS=$((FAILS + 1))
  fi
}

make_stub_go() {
  local bin_dir="$1" log_file="$2"
  cat >"$bin_dir/go" <<EOF
#!/usr/bin/env bash
{
  echo "GO_CALLED=1"
  echo "LD_LIBRARY_PATH=\${LD_LIBRARY_PATH:-}"
  echo "CGO_LDFLAGS=\${CGO_LDFLAGS:-}"
} >>"$log_file"
exit 0
EOF
  chmod +x "$bin_dir/go"
}

make_stub_python3() {
  local bin_dir="$1" log_file="$2" exit_code="$3"
  cat >"$bin_dir/python3" <<EOF
#!/usr/bin/env bash
echo "\$*" >>"$log_file"
exit $exit_code
EOF
  chmod +x "$bin_dir/python3"
}

setup_case() {
  local name="$1"
  CASE_DIR="$TMP_ROOT/$name"
  CASE_BIN="$CASE_DIR/bin"
  CASE_HOME="$CASE_DIR/home"
  CASE_EVIDENCE="$CASE_DIR/evidence"
  PY_LOG="$CASE_DIR/python3.log"
  GO_LOG="$CASE_DIR/go.log"
  mkdir -p "$CASE_BIN" "$CASE_HOME" "$CASE_EVIDENCE"
  cp -a "$CLEAN_BIN/." "$CASE_BIN/"
}

OUT=""
RC=0
run_hermetic() {
  local extra_ld="${1:-}"
  shift
  set +e
  if [[ -n "$extra_ld" ]]; then
    OUT=$(cd "$REPO_ROOT" && env -i \
      HOME="$CASE_HOME" \
      PATH="$CASE_BIN" \
      LD_LIBRARY_PATH="$extra_ld" \
      PACT_EVIDENCE_DIR="$CASE_EVIDENCE" \
      PACT_CHECK_NO_PATH_STITCH=1 \
      "$SCRIPT" "$@" 2>&1)
  else
    OUT=$(cd "$REPO_ROOT" && env -i \
      HOME="$CASE_HOME" \
      PATH="$CASE_BIN" \
      PACT_EVIDENCE_DIR="$CASE_EVIDENCE" \
      PACT_CHECK_NO_PATH_STITCH=1 \
      "$SCRIPT" "$@" 2>&1)
  fi
  RC=$?
  set -e
}

# --- Case 1: python3 provisions pinned FFI and exports LD_LIBRARY_PATH + CGO_LDFLAGS ---
echo "== Case 1: provisions pinned FFI and exports env to Go steps =="
setup_case case1
make_stub_go "$CASE_BIN" "$GO_LOG"
make_stub_python3 "$CASE_BIN" "$PY_LOG" 0
run_hermetic "" --services altctl --role consumer
assert_rc_zero "$RC" "pact-check.sh exits 0 when python3 and go stubs succeed"
py_calls="$(cat "$PY_LOG" 2>/dev/null || true)"
go_env="$(cat "$GO_LOG" 2>/dev/null || true)"
assert_contains "$py_calls" "$REPO_ROOT/scripts/ci/pact-artifacts.py --type ffi" \
  "invokes ci/pact-artifacts.py --type ffi"
assert_contains "$go_env" "LD_LIBRARY_PATH=$CASE_HOME/.pact/lib" \
  "Go step sees LD_LIBRARY_PATH set to \$HOME/.pact/lib"
assert_contains "$go_env" "CGO_LDFLAGS=-L$CASE_HOME/.pact/lib" \
  "Go step sees CGO_LDFLAGS=-L\$HOME/.pact/lib"

# --- Case 2: pre-set LD_LIBRARY_PATH with fake libpact_ffi.so does not take precedence ---
echo "== Case 2: pre-existing LD_LIBRARY_PATH does not bypass pin or take precedence =="
setup_case case2
make_stub_go "$CASE_BIN" "$GO_LOG"
make_stub_python3 "$CASE_BIN" "$PY_LOG" 0
OTHER_LIB="$CASE_DIR/other-lib"
mkdir -p "$OTHER_LIB"
: >"$OTHER_LIB/libpact_ffi.so"
run_hermetic "$OTHER_LIB" --services altctl --role consumer
assert_rc_zero "$RC" "pact-check.sh exits 0 with pre-existing LD_LIBRARY_PATH"
py_calls="$(cat "$PY_LOG" 2>/dev/null || true)"
go_env="$(cat "$GO_LOG" 2>/dev/null || true)"
assert_contains "$py_calls" "ci/pact-artifacts.py --type ffi" \
  "still runs pact-artifacts.py --type ffi even when LD_LIBRARY_PATH has libpact_ffi.so"
assert_contains "$go_env" "LD_LIBRARY_PATH=$CASE_HOME/.pact/lib:$OTHER_LIB" \
  "prepends \$HOME/.pact/lib ahead of pre-existing LD_LIBRARY_PATH"
assert_contains "$go_env" "CGO_LDFLAGS=-L$CASE_HOME/.pact/lib" \
  "exports CGO_LDFLAGS=-L\$HOME/.pact/lib"

# --- Case 3a: provisioning failure (python3 exits 1) stops with error and does not skip Go ---
echo "== Case 3a: python3 failure stops script without skipping Go steps =="
setup_case case3a
make_stub_go "$CASE_BIN" "$GO_LOG"
make_stub_python3 "$CASE_BIN" "$PY_LOG" 1
run_hermetic "" --services altctl --role consumer
assert_rc_nonzero "$RC" "pact-check.sh exits non-zero when python3 provisioning fails"
assert_contains "$OUT" "failed to provision" \
  "stderr/output names provisioning failure"
assert_not_contains "$OUT" "SKIP" \
  "does not mark Go steps as SKIP when provisioning fails"
TESTS=$((TESTS + 1))
if [[ ! -s "$GO_LOG" ]]; then
  echo "  PASS: Go step is not executed when provisioning fails"
else
  echo "  FAIL: Go step is not executed when provisioning fails"
  FAILS=$((FAILS + 1))
fi

# --- Case 3b: missing python3 while go is present stops with error and does not skip Go ---
echo "== Case 3b: missing python3 while go is present stops script without skipping Go =="
setup_case case3b
make_stub_go "$CASE_BIN" "$GO_LOG"
run_hermetic "" --services altctl --role consumer
assert_rc_nonzero "$RC" "pact-check.sh exits non-zero when python3 is missing"
assert_contains "$OUT" "python3 is missing" \
  "stderr/output names missing python3"
assert_not_contains "$OUT" "SKIP" \
  "does not mark Go steps as SKIP when python3 is missing"
TESTS=$((TESTS + 1))
if [[ ! -s "$GO_LOG" ]]; then
  echo "  PASS: Go step is not executed when python3 is missing"
else
  echo "  FAIL: Go step is not executed when python3 is missing"
  FAILS=$((FAILS + 1))
fi

# --- Case 4: --dry-run does not invoke provisioning ---
echo "== Case 4: --dry-run skips provisioning =="
setup_case case4
make_stub_go "$CASE_BIN" "$GO_LOG"
make_stub_python3 "$CASE_BIN" "$PY_LOG" 1
run_hermetic "" --dry-run --services altctl --role consumer
assert_rc_zero "$RC" "pact-check.sh --dry-run exits 0 without provisioning"
assert_contains "$OUT" "WOULD RUN: Go: altctl sovereign consumer" \
  "--dry-run reports WOULD RUN for selected Go step"
TESTS=$((TESTS + 1))
if [[ ! -s "$PY_LOG" ]]; then
  echo "  PASS: --dry-run does not invoke python3 provisioning"
else
  echo "  FAIL: --dry-run does not invoke python3 provisioning"
  FAILS=$((FAILS + 1))
fi

# --- Case 5: no go on PATH skips Go steps without provisioning ---
echo "== Case 5: missing go skips Go steps without provisioning =="
setup_case case5
make_stub_python3 "$CASE_BIN" "$PY_LOG" 1
run_hermetic "" --services altctl --role consumer
assert_rc_zero "$RC" "pact-check.sh exits 0 when go is absent"
assert_contains "$OUT" "SKIP: Go: altctl sovereign consumer (missing go toolchain)" \
  "reports Go step as skipped for missing go toolchain"
TESTS=$((TESTS + 1))
if [[ ! -s "$PY_LOG" ]]; then
  echo "  PASS: does not invoke python3 provisioning when go is absent"
else
  echo "  FAIL: does not invoke python3 provisioning when go is absent"
  FAILS=$((FAILS + 1))
fi

if [[ $FAILS -gt 0 ]]; then
  echo ""
  echo "FAILED: $FAILS of $TESTS assertions failed"
  exit 1
fi

echo ""
echo "ALL PASSED ($TESTS assertions)"
