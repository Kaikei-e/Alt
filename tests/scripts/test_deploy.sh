#!/usr/bin/env bash
# Tests for scripts/deploy.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/lib.sh"

SUT="$REPO_ROOT/scripts/deploy.sh"

export_sut_env() {
  export PRE_DEPLOY_VERIFY_SCRIPT="$STUB_BIN/pre-deploy-verify.sh"
  export PACT_BROKER_BIN="$STUB_BIN/pact-broker-cli"
  export DOCKER_BIN="$STUB_BIN/docker"
  export CURL_BIN="$STUB_BIN/curl"
  export GIT_BIN="$(command -v git)"
  # Point the SUT's notion of the repo at our sandboxed git repo so
  # git rev-parse works and .deploy-prev writes don't pollute the real tree.
  export DEPLOY_REPO_ROOT="$DEPLOY_WORKDIR"
  export PACT_BROKER_BASE_URL="http://127.0.0.1:9292"
  export PACT_BROKER_USERNAME="pact"
  export PACT_BROKER_PASSWORD="test-pw"
  # Use a very small layer order for tests to keep invocations manageable.
  export DEPLOY_LAYERS_OVERRIDE="step-ca|kratos|alt-backend|news-creator|nginx"
  # Disable actual global smoke curl wait (stub handles it).
  export SMOKE_WAIT_SECONDS="0"
  export HEALTHCHECK_TIMEOUT_SECONDS="5"
  # List the 14 pacticipants (same as production).
  export DEPLOY_PACTICIPANTS="alt-backend pre-processor search-indexer mq-hub rag-orchestrator recap-worker recap-subworker recap-evaluator news-creator tag-generator tts-speaker acolyte-orchestrator alt-butterfly-facade auth-hub"
}

tc_refuses_when_verify_fails() {
  export_sut_env
  make_stub "pre-deploy-verify.sh" 3 "broker heartbeat failed"
  make_stub docker 0 ""
  make_stub curl 0 ""
  make_stub "pact-broker-cli" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "must refuse deploy when verify fails" || return 1
  assert_contains "$run_output" "pre-deploy-verify" "must mention verify in error" || return 1
  # docker compose up must NOT have run
  if grep -q 'up -d' "$STUB_LOG"; then
    echo "  FAIL: docker compose up should not have run"
    return 1
  fi
  # record-deployment must NOT have run
  if grep -q "record-deployment" "$STUB_LOG"; then
    echo "  FAIL: record-deployment should not have run"
    return 1
  fi
}

tc_recreates_services_in_layered_order() {
  export_sut_env
  make_stub "pre-deploy-verify.sh" 0 ""
  # docker stub: respond to compose ps --format 'json' with a healthy state.
  make_conditional_stub docker '
    if [[ "$1" == "compose" ]]; then
      case "$*" in
        *"ps"*)
          echo "running healthy"
          exit 0 ;;
        *"up -d"*|*"pull"*|*"build"*)
          exit 0 ;;
        *) exit 0 ;;
      esac
    fi
    exit 0'
  make_stub curl 0 "ok"
  make_stub "pact-broker-cli" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_eq "$rc" "0" "happy path must exit 0" || { echo "$run_output"; return 1; }
  # assert layered order in the stub log — services must appear in the given order
  assert_order_in_log "step-ca" "kratos" "alt-backend" "news-creator" "nginx" || return 1
}

tc_rolls_back_on_healthcheck_failure() {
  export_sut_env
  make_stub "pre-deploy-verify.sh" 0 ""
  # docker ps returns "unhealthy" for alt-backend always -> healthcheck timeout.
  make_conditional_stub docker '
    if [[ "$1" == "compose" ]]; then
      if [[ "$*" == *"ps --services --filter status=running"* ]]; then
        exit 0
      fi
      if [[ "$*" == *"ps"* && "$*" == *"alt-backend"* ]]; then
        echo "unhealthy"
        exit 0
      fi
      if [[ "$*" == *"ps"* ]]; then
        echo "healthy"
        exit 0
      fi
      exit 0
    fi
    exit 0'
  make_stub curl 0 "ok"
  make_stub "pact-broker-cli" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when healthcheck never turns healthy" || return 1
  assert_contains "$run_output" "rolling back" "must mention rollback" || return 1
  # record-deployment must NOT run on failure
  if grep -q "record-deployment" "$STUB_LOG"; then
    echo "  FAIL: record-deployment should not have run after rollback"
    return 1
  fi
}

tc_records_deployment_on_success() {
  export_sut_env
  make_stub "pre-deploy-verify.sh" 0 ""
  make_conditional_stub docker '
    if [[ "$1" == "compose" ]]; then
      if [[ "$*" == *"ps"* ]]; then echo "healthy"; exit 0; fi
      exit 0
    fi
    exit 0'
  make_stub curl 0 "ok"
  make_stub "pact-broker-cli" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_eq "$rc" "0" "happy path must exit 0" || { echo "$run_output"; return 1; }
  count=$(grep -c "record-deployment" "$STUB_LOG" || true)
  assert_eq "$count" "14" "must record-deployment for all 14 pacticipants" || return 1
}

tc_dry_run_does_not_call_docker_up() {
  export_sut_env
  make_stub "pre-deploy-verify.sh" 0 ""
  make_stub docker 0 ""
  make_stub curl 0 "ok"
  make_stub "pact-broker-cli" 0 ""

  run_output=$("$SUT" --dry-run production 2>&1); rc=$?
  assert_eq "$rc" "0" "dry-run must succeed" || { echo "$run_output"; return 1; }
  if grep -q "up -d" "$STUB_LOG"; then
    echo "  FAIL: dry-run must not call docker compose up -d"
    return 1
  fi
  if grep -q "record-deployment" "$STUB_LOG"; then
    echo "  FAIL: dry-run must not record deployment"
    return 1
  fi
}

tc_skip_verify_bypasses_gate() {
  export_sut_env
  # Intentionally make the gate fail — --skip-verify must bypass it.
  make_stub "pre-deploy-verify.sh" 3 "would fail"
  make_conditional_stub docker '
    if [[ "$1" == "compose" ]]; then
      if [[ "$*" == *"ps"* ]]; then echo "healthy"; exit 0; fi
      exit 0
    fi
    exit 0'
  make_stub curl 0 "ok"
  make_stub "pact-broker-cli" 0 ""

  run_output=$("$SUT" --skip-verify production 2>&1); rc=$?
  assert_eq "$rc" "0" "--skip-verify must bypass gate" || { echo "$run_output"; return 1; }
  assert_contains "$run_output" "skipping pre-deploy" "must warn about bypass" || return 1
}

main() {
  echo "deploy tests"
  run_case "refuses to deploy when pre-deploy-verify fails" tc_refuses_when_verify_fails
  run_case "recreates services in layered order" tc_recreates_services_in_layered_order
  run_case "rolls back on healthcheck failure and does not record" tc_rolls_back_on_healthcheck_failure
  run_case "records deployment only after full success (14 times)" tc_records_deployment_on_success
  run_case "--dry-run does not call docker compose up -d" tc_dry_run_does_not_call_docker_up
  run_case "--skip-verify bypasses gate with warning" tc_skip_verify_bypasses_gate
  summary
}

main "$@"
