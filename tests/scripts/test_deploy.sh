#!/usr/bin/env bash
# Tests for scripts/deploy.sh — c2quay-backed thin wrapper.
#
# The wrapper chains three steps in order:
#   1. scripts/pact-check.sh --broker    (publishes pacts; c2quay does not)
#   2. c2quay deploy --env <env> --config ./c2quay.yml
#   3. scripts/record-remote-pacticipant.sh <env>   (tts-speaker, remote host)
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
  export RECORD_REMOTE_SCRIPT="$STUB_BIN/record-remote-pacticipant.sh"
  export C2QUAY_CONFIG="$SANDBOX/c2quay.yml"
  : >"$C2QUAY_CONFIG"
  export PACT_BROKER_USERNAME="pact"
  export PACT_BROKER_PASSWORD="test-pw"
}

tc_runs_all_three_steps_in_order_on_success() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "record-remote-pacticipant.sh" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_eq "$rc" "0" "happy path must exit 0" || { echo "$run_output"; return 1; }
  assert_order_in_log "pact-check.sh --broker" "c2quay deploy" "record-remote-pacticipant.sh production" || return 1
}

tc_aborts_when_pact_check_fails() {
  export_sut_env
  make_stub "pact-check.sh" 1 "contract regression"
  make_stub c2quay 0 ""
  make_stub "record-remote-pacticipant.sh" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when pact-check fails" || return 1
  if grep -q "c2quay" "$STUB_LOG"; then
    echo "  FAIL: c2quay must not run when pact-check fails"
    return 1
  fi
  if grep -q "record-remote-pacticipant" "$STUB_LOG"; then
    echo "  FAIL: record-remote-pacticipant must not run when pact-check fails"
    return 1
  fi
}

tc_aborts_when_c2quay_fails() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 2 "can-i-deploy blocked"
  make_stub "record-remote-pacticipant.sh" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when c2quay fails" || return 1
  if grep -q "record-remote-pacticipant" "$STUB_LOG"; then
    echo "  FAIL: record-remote-pacticipant must not run after c2quay failure"
    return 1
  fi
}

tc_aborts_when_record_remote_fails() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "record-remote-pacticipant.sh" 7 "tts-speaker record-deployment failed"

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when remote record fails" || return 1
}

tc_defaults_env_to_production() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "record-remote-pacticipant.sh" 0 ""

  "$SUT" >/dev/null 2>&1
  if ! stub_called_with c2quay deploy --env production --config "$C2QUAY_CONFIG"; then
    echo "  FAIL: c2quay must be called with --env production by default"
    cat "$STUB_LOG"
    return 1
  fi
  if ! stub_called_with "record-remote-pacticipant.sh" production; then
    echo "  FAIL: record-remote-pacticipant must receive production as arg"
    return 1
  fi
}

tc_passes_custom_env_arg() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "record-remote-pacticipant.sh" 0 ""

  "$SUT" staging >/dev/null 2>&1
  if ! stub_called_with c2quay deploy --env staging --config "$C2QUAY_CONFIG"; then
    echo "  FAIL: c2quay must receive staging env"
    cat "$STUB_LOG"
    return 1
  fi
  if ! stub_called_with "record-remote-pacticipant.sh" staging; then
    echo "  FAIL: record-remote-pacticipant must receive staging arg"
    return 1
  fi
}

main() {
  echo "deploy tests (c2quay wrapper)"
  run_case "chains pact-check → c2quay → record-remote on success" tc_runs_all_three_steps_in_order_on_success
  run_case "aborts without running c2quay or remote when pact-check fails" tc_aborts_when_pact_check_fails
  run_case "aborts without running remote when c2quay fails" tc_aborts_when_c2quay_fails
  run_case "exits non-zero when remote record-deployment fails" tc_aborts_when_record_remote_fails
  run_case "defaults env to production when no arg given" tc_defaults_env_to_production
  run_case "passes custom env arg through to c2quay and remote" tc_passes_custom_env_arg
  summary
}

main "$@"
