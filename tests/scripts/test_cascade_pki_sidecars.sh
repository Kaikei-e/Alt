#!/usr/bin/env bash
# Tests for the retired scripts/cascade-pki-sidecars.sh tombstone.
# Wave 4 Pattern B cutover left zero netns-sharing pki sidecars, so
# invoking this script is always a bug: the caller still thinks a
# shared-netns reverse-proxy sidecar exists.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/lib.sh"

SUT="$REPO_ROOT/scripts/cascade-pki-sidecars.sh"

tc_tombstone_exits_nonzero() {
  run_output=$("$SUT" 2>&1); rc=$?
  assert_ne "$rc" "0" "retired cascade must exit non-zero" || { echo "$run_output"; return 1; }
  assert_contains "$run_output" "retired" "must say it is retired" || return 1
}

tc_tombstone_has_no_netns_array() {
  if grep -q '^NETNS_SIDECARS=' "$SUT"; then
    echo "  FAIL: tombstone must not keep a NETNS_SIDECARS array"
    return 1
  fi
}

tc_tombstone_does_not_call_compose() {
  export DOCKER_BIN="$STUB_BIN/docker"
  make_stub docker 0 ""
  "$SUT" >/dev/null 2>&1 || true
  if grep -q "compose" "$STUB_LOG" 2>/dev/null; then
    echo "  FAIL: tombstone must not invoke docker compose"
    cat "$STUB_LOG"
    return 1
  fi
}

main() {
  echo "cascade-pki-sidecars.sh tombstone tests"
  run_case "exits non-zero" tc_tombstone_exits_nonzero
  run_case "has no NETNS_SIDECARS array" tc_tombstone_has_no_netns_array
  run_case "does not call docker compose" tc_tombstone_does_not_call_compose
  summary
}

main "$@"
