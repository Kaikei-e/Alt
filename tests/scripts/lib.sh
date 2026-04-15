#!/usr/bin/env bash
# Minimal bash test harness for scripts/*.sh.
# Keeps zero external deps — no bats, no shunit.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export REPO_ROOT

PASS=0
FAIL=0
FAILED_CASES=()
CURRENT_CASE=""

setup_sandbox() {
  SANDBOX="$(mktemp -d -t alt-deploy-test.XXXXXX)"
  STUB_BIN="$SANDBOX/bin"
  mkdir -p "$STUB_BIN"
  STUB_LOG="$SANDBOX/log"
  : >"$STUB_LOG"
  # Preserve real PATH tail so built-ins like bash, sed resolve.
  export PATH="$STUB_BIN:$PATH"
  export SANDBOX STUB_BIN STUB_LOG
  # Seed a working git repo for scripts that call `git rev-parse`.
  (
    cd "$SANDBOX"
    git init -q
    git -c user.email=t@t -c user.name=t commit -q --allow-empty -m init
  )
  export DEPLOY_WORKDIR="$SANDBOX"
}

teardown_sandbox() {
  [[ -n "${SANDBOX:-}" && -d "$SANDBOX" ]] && rm -rf "$SANDBOX"
}

# make_stub NAME "exit_code" ["stdout"]
# Creates a fake executable at $STUB_BIN/NAME that records argv and exits with given code.
make_stub() {
  local name="$1" code="${2:-0}" out="${3:-}"
  local path="$STUB_BIN/$name"
  {
    echo '#!/usr/bin/env bash'
    echo "echo \"[stub] $name \$*\" >> \"$STUB_LOG\""
    [[ -n "$out" ]] && echo "printf '%s\n' \"$out\""
    echo "exit $code"
  } >"$path"
  chmod +x "$path"
}

# make_conditional_stub NAME dispatch_body
# dispatch_body is a bash snippet receiving "$@" that decides exit/output.
make_conditional_stub() {
  local name="$1"; shift
  local body="$*"
  local path="$STUB_BIN/$name"
  {
    echo '#!/usr/bin/env bash'
    echo "echo \"[stub] $name \$*\" >> \"$STUB_LOG\""
    echo "$body"
  } >"$path"
  chmod +x "$path"
}

stub_called_with() {
  local name="$1"; shift
  local needle="$*"
  grep -F "[stub] $name $needle" "$STUB_LOG" >/dev/null
}

stub_call_count() {
  local name="$1"
  grep -cF "[stub] $name " "$STUB_LOG" || true
}

assert_eq() {
  local actual="$1" expected="$2" msg="${3:-assert_eq}"
  if [[ "$actual" != "$expected" ]]; then
    echo "  FAIL: $msg (expected='$expected' actual='$actual')"
    return 1
  fi
}

assert_ne() {
  local actual="$1" forbidden="$2" msg="${3:-assert_ne}"
  if [[ "$actual" == "$forbidden" ]]; then
    echo "  FAIL: $msg (unexpected='$forbidden')"
    return 1
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" msg="${3:-assert_contains}"
  if [[ "$haystack" != *"$needle"* ]]; then
    echo "  FAIL: $msg (needle='$needle' not found)"
    echo "  ---- haystack ----"
    echo "$haystack"
    echo "  ------------------"
    return 1
  fi
}

assert_order_in_log() {
  # assert_order_in_log first second third ...
  local prev_line=0
  for token in "$@"; do
    local line
    line=$(grep -nF "$token" "$STUB_LOG" | head -n1 | cut -d: -f1)
    if [[ -z "$line" ]]; then
      echo "  FAIL: assert_order_in_log — token '$token' not found in stub log"
      return 1
    fi
    if (( line < prev_line )); then
      echo "  FAIL: assert_order_in_log — '$token' at line $line appeared before previous token (line $prev_line)"
      return 1
    fi
    prev_line=$line
  done
}

begin_case() {
  CURRENT_CASE="$1"
  setup_sandbox
  printf '  - %s ... ' "$CURRENT_CASE"
}

end_case_ok() {
  PASS=$((PASS+1))
  echo "ok"
  teardown_sandbox
}

end_case_fail() {
  FAIL=$((FAIL+1))
  FAILED_CASES+=("$CURRENT_CASE")
  echo "FAIL"
  # Keep sandbox for diagnostics on failure.
  echo "  (sandbox kept at $SANDBOX)"
}

# Convenience wrapper: runs a test function, captures errexit cleanly.
run_case() {
  local name="$1" fn="$2"
  begin_case "$name"
  local ok=1
  if "$fn"; then
    end_case_ok
  else
    ok=0
    end_case_fail
  fi
  return 0
}

summary() {
  echo ""
  echo "============================="
  echo " Test Summary"
  echo "============================="
  echo "  Passed: $PASS"
  echo "  Failed: $FAIL"
  if (( FAIL > 0 )); then
    echo "  Failed cases:"
    for c in "${FAILED_CASES[@]}"; do
      echo "    - $c"
    done
    return 1
  fi
  return 0
}
