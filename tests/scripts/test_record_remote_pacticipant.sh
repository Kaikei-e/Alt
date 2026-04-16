#!/usr/bin/env bash
# Tests for scripts/record-remote-pacticipant.sh.
#
# Records a pact-broker deployment for tts-speaker (lives on a separate GPU
# host, so c2quay cannot roll it out but the broker matrix still needs the
# release flag set).
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/lib.sh"

SUT="$REPO_ROOT/scripts/record-remote-pacticipant.sh"

export_sut_env() {
  export PACT_BROKER_BIN="$STUB_BIN/pact-broker-cli"
  export PACT_BROKER_BASE_URL="http://127.0.0.1:9292"
  export PACT_BROKER_USERNAME="pact"
  export PACT_BROKER_PASSWORD="test-pw"
  # Point at the sandbox git repo so `git rev-parse --short HEAD` resolves.
  export DEPLOY_REPO_ROOT="$DEPLOY_WORKDIR"
}

tc_records_tts_speaker_once_with_version_and_env() {
  export_sut_env
  make_stub "pact-broker-cli" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_eq "$rc" "0" "happy path must exit 0" || { echo "$run_output"; return 1; }
  count=$(grep -c "record-deployment" "$STUB_LOG" || true)
  assert_eq "$count" "1" "must call record-deployment exactly once" || return 1
  if ! grep -qE "pacticipant[= ]tts-speaker" "$STUB_LOG"; then
    echo "  FAIL: must target the tts-speaker pacticipant"
    cat "$STUB_LOG"
    return 1
  fi
  if ! grep -qE "environment[= ]production" "$STUB_LOG"; then
    echo "  FAIL: must pass --environment production"
    cat "$STUB_LOG"
    return 1
  fi
}

tc_defaults_env_to_production() {
  export_sut_env
  make_stub "pact-broker-cli" 0 ""

  "$SUT" >/dev/null 2>&1
  if ! grep -qE "environment[= ]production" "$STUB_LOG"; then
    echo "  FAIL: when no arg given, must default to production"
    cat "$STUB_LOG"
    return 1
  fi
}

tc_uses_custom_env_arg() {
  export_sut_env
  make_stub "pact-broker-cli" 0 ""

  "$SUT" staging >/dev/null 2>&1
  if ! grep -qE "environment[= ]staging" "$STUB_LOG"; then
    echo "  FAIL: must pass --environment staging"
    cat "$STUB_LOG"
    return 1
  fi
}

tc_propagates_broker_cli_failure() {
  export_sut_env
  make_stub "pact-broker-cli" 1 "broker 500"

  "$SUT" production >/dev/null 2>&1; rc=$?
  assert_ne "$rc" "0" "must exit non-zero when pact-broker-cli fails" || return 1
}

tc_version_matches_git_head_short_sha() {
  export_sut_env
  make_stub "pact-broker-cli" 0 ""

  "$SUT" production >/dev/null 2>&1
  local expected_sha
  expected_sha=$(cd "$DEPLOY_WORKDIR" && git rev-parse --short HEAD)
  if ! grep -qE "version[= ]${expected_sha}" "$STUB_LOG"; then
    echo "  FAIL: version must be the sandbox git short SHA ($expected_sha)"
    cat "$STUB_LOG"
    return 1
  fi
}

main() {
  echo "record-remote-pacticipant tests"
  run_case "records tts-speaker once with version and env" tc_records_tts_speaker_once_with_version_and_env
  run_case "defaults env to production when no arg given" tc_defaults_env_to_production
  run_case "passes custom env arg through" tc_uses_custom_env_arg
  run_case "propagates pact-broker-cli failure as non-zero exit" tc_propagates_broker_cli_failure
  run_case "uses git rev-parse --short HEAD as the version" tc_version_matches_git_head_short_sha
  summary
}

main "$@"
