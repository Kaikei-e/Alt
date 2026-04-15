#!/usr/bin/env bash
# Tests for scripts/pre-deploy-verify.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/lib.sh"

SUT="$REPO_ROOT/scripts/pre-deploy-verify.sh"

# Common env for the script-under-test.
# We stub pact-check.sh via PACT_CHECK_SCRIPT, the broker CLI via PACT_BROKER_BIN,
# and curl via the sandbox PATH.
export_sut_env() {
  export PACT_BROKER_BASE_URL="http://127.0.0.1:9292"
  export PACT_BROKER_USERNAME="pact"
  export PACT_BROKER_PASSWORD="test-pw"
  export PACT_CHECK_SCRIPT="$STUB_BIN/pact-check.sh"
  export PACT_BROKER_BIN="$STUB_BIN/pact-broker"
  export TARGET_ENV="production"
  export SKIP_CREATE_ENV_ON_SEED="0"
}

tc_fails_when_broker_unreachable() {
  export_sut_env
  # curl stub that always fails heartbeat.
  make_stub curl 7 ""
  make_stub "pact-check.sh" 0 ""
  make_stub "pact-broker" 0 ""

  run_output=$("$SUT" 2>&1); rc=$?
  assert_ne "$rc" "0" "should fail when broker heartbeat unreachable" || return 1
  assert_contains "$run_output" "broker" "output must mention broker" || return 1
}

tc_fails_when_pact_check_fails() {
  export_sut_env
  make_stub curl 0 '{"ok":true}'
  make_stub "pact-check.sh" 1 "contract regression detected"
  make_stub "pact-broker" 0 ""

  run_output=$("$SUT" 2>&1); rc=$?
  assert_ne "$rc" "0" "should fail when pact-check fails" || return 1
  assert_contains "$run_output" "contract" "must mention contract regression" || return 1
}

tc_fails_when_can_i_deploy_fails_for_any_pacticipant() {
  export_sut_env
  make_stub curl 0 '{"ok":true}'
  make_stub "pact-check.sh" 0 ""
  # Fail for alt-backend only.
  make_conditional_stub "pact-broker" '
    case "$1" in
      can-i-deploy)
        for arg in "$@"; do
          if [[ "$arg" == "alt-backend" ]]; then exit 1; fi
        done
        exit 0 ;;
      create-environment) exit 0 ;;
      *) exit 0 ;;
    esac'

  run_output=$("$SUT" 2>&1); rc=$?
  assert_ne "$rc" "0" "should fail when alt-backend fails can-i-deploy" || return 1
  assert_contains "$run_output" "alt-backend" "must name failing service" || return 1
}

tc_happy_path_runs_create_env_and_all_pacticipants() {
  export_sut_env
  make_stub curl 0 '{"ok":true}'
  make_stub "pact-check.sh" 0 ""
  make_stub "pact-broker" 0 ""

  run_output=$("$SUT" 2>&1); rc=$?
  assert_eq "$rc" "0" "happy path must exit 0" || { echo "$run_output"; return 1; }
  # create-environment must be invoked once
  count=$(grep -c "create-environment" "$STUB_LOG" || true)
  [[ "$count" -ge 1 ]] || { echo "  FAIL: create-environment not invoked"; return 1; }
  # can-i-deploy must be invoked for all 14 pacticipants
  count=$(grep -c "can-i-deploy" "$STUB_LOG" || true)
  assert_eq "$count" "14" "must run can-i-deploy 14 times (one per pacticipant)" || return 1
}

tc_create_environment_is_idempotent() {
  export_sut_env
  make_stub curl 0 '{"ok":true}'
  make_stub "pact-check.sh" 0 ""
  # pact-broker create-environment returns 1 the first time (seeded already) but script must ignore.
  make_conditional_stub "pact-broker" '
    if [[ "$1" == "create-environment" ]]; then
      # Simulate "already exists" by exiting non-zero. pre-deploy-verify must tolerate.
      exit 1
    fi
    exit 0'

  run_output=$("$SUT" 2>&1); rc=$?
  assert_eq "$rc" "0" "script must not abort when create-environment returns non-zero (already exists)" || { echo "$run_output"; return 1; }
}

main() {
  echo "pre-deploy-verify tests"
  run_case "fails fast when pact broker heartbeat is unreachable" tc_fails_when_broker_unreachable
  run_case "exits non-zero when pact-check.sh --broker fails" tc_fails_when_pact_check_fails
  run_case "exits non-zero when any pacticipant fails can-i-deploy" tc_fails_when_can_i_deploy_fails_for_any_pacticipant
  run_case "happy path runs create-environment and 14 can-i-deploy" tc_happy_path_runs_create_env_and_all_pacticipants
  run_case "create-environment failure is tolerated (idempotent)" tc_create_environment_is_idempotent
  summary
}

main "$@"
