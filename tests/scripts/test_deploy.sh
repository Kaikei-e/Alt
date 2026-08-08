#!/usr/bin/env bash
# Tests for scripts/deploy.sh — c2quay-backed thin wrapper.
#
# The wrapper chains three steps in order:
#   1. scripts/pact-check.sh --broker    (publishes pacts; c2quay does not)
#   2. c2quay deploy --env <env> --config ./c2quay.yml
#   3. scripts/cascade-pki-sidecars.sh   (netns-sharing sidecar cascade)
#
# Any step failing aborts the chain without running the next.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/lib.sh"

SUT="$REPO_ROOT/scripts/deploy.sh"

export_sut_env() {
  export PACT_CHECK_SCRIPT="$STUB_BIN/pact-check.sh"
  export C2QUAY_BIN="$STUB_BIN/c2quay"
  export CASCADE_SCRIPT="$STUB_BIN/cascade-pki-sidecars.sh"
  export C2QUAY_CONFIG="$SANDBOX/c2quay.yml"
  : >"$C2QUAY_CONFIG"
  export PACT_BROKER_USERNAME="pact"
  export PACT_BROKER_PASSWORD="test-pw"
}

tc_runs_all_three_steps_in_order_on_success() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "cascade-pki-sidecars.sh" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_eq "$rc" "0" "happy path must exit 0" || { echo "$run_output"; return 1; }
  assert_order_in_log "pact-check.sh --broker" "c2quay deploy" "cascade-pki-sidecars.sh" || return 1
}

tc_aborts_when_pact_check_fails() {
  export_sut_env
  make_stub "pact-check.sh" 1 "contract regression"
  make_stub c2quay 0 ""
  make_stub "cascade-pki-sidecars.sh" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when pact-check fails" || return 1
  if grep -q "c2quay" "$STUB_LOG"; then
    echo "  FAIL: c2quay must not run when pact-check fails"
    return 1
  fi
  if grep -q "cascade-pki-sidecars" "$STUB_LOG"; then
    echo "  FAIL: cascade-pki-sidecars must not run when pact-check fails"
    return 1
  fi
}

tc_aborts_when_c2quay_fails() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 2 "can-i-deploy blocked"
  make_stub "cascade-pki-sidecars.sh" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when c2quay fails" || return 1
  if grep -q "cascade-pki-sidecars" "$STUB_LOG"; then
    echo "  FAIL: cascade-pki-sidecars must not run after c2quay failure"
    return 1
  fi
}

tc_aborts_when_cascade_fails() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "cascade-pki-sidecars.sh" 5 "sidecar recreate failed"

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when cascade-pki-sidecars fails" || return 1
}

tc_defaults_env_to_production() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "cascade-pki-sidecars.sh" 0 ""

  "$SUT" >/dev/null 2>&1
  if ! stub_called_with c2quay deploy --env production --config "$C2QUAY_CONFIG"; then
    echo "  FAIL: c2quay must be called with --env production by default"
    cat "$STUB_LOG"
    return 1
  fi
  if ! stub_called_with "cascade-pki-sidecars.sh"; then
    echo "  FAIL: cascade-pki-sidecars.sh must be invoked"
    return 1
  fi
}

tc_passes_custom_env_arg() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "cascade-pki-sidecars.sh" 0 ""

  "$SUT" staging >/dev/null 2>&1
  if ! stub_called_with c2quay deploy --env staging --config "$C2QUAY_CONFIG"; then
    echo "  FAIL: c2quay must receive staging env"
    cat "$STUB_LOG"
    return 1
  fi
}

main() {
  echo "deploy tests (c2quay wrapper)"
  run_case "chains pact-check → c2quay → cascade-pki-sidecars on success" tc_runs_all_three_steps_in_order_on_success
  run_case "aborts without running c2quay or cascade when pact-check fails" tc_aborts_when_pact_check_fails
  run_case "aborts without running cascade when c2quay fails" tc_aborts_when_c2quay_fails
  run_case "exits non-zero when cascade-pki-sidecars fails" tc_aborts_when_cascade_fails
  run_case "defaults env to production when no arg given" tc_defaults_env_to_production
  run_case "passes custom env arg through to c2quay" tc_passes_custom_env_arg
  summary
}

main "$@"
