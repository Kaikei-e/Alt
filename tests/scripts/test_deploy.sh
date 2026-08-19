#!/usr/bin/env bash
# Tests for scripts/deploy.sh — c2quay-backed thin wrapper.
#
# The wrapper chains:
#   1. scripts/check-compose-variables.sh (env render gate)
#   2. scripts/pact-check.sh --broker     (publishes pacts; c2quay does not)
#   3. leftover pki-agent stop/rm + zero gate (project=alt, before parent up)
#   4. c2quay deploy --env <env> --config ./c2quay.yml
#
# Wave 4 retired scripts/cascade-pki-sidecars.sh. There is no netns-sharing
# pki sidecar left to cascade; deploy must not invoke that tombstone.
# Leftover cleanup is the exact-label sweep, never `up --remove-orphans`.
#
# Any step failing aborts the chain without running the next.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/lib.sh"

SUT="$REPO_ROOT/scripts/deploy.sh"
SWEEP="$REPO_ROOT/scripts/retire-alt-pki-agent-leftovers.sh"

export_sut_env() {
  export COMPOSE_VARS_SCRIPT="$STUB_BIN/check-compose-variables.sh"
  export PACT_CHECK_SCRIPT="$STUB_BIN/pact-check.sh"
  export C2QUAY_BIN="$STUB_BIN/c2quay"
  export CASCADE_SCRIPT="$STUB_BIN/cascade-pki-sidecars.sh"
  export LEFTOVER_SWEEP_SCRIPT="$STUB_BIN/retire-alt-pki-agent-leftovers.sh"
  export C2QUAY_CONFIG="$SANDBOX/c2quay.yml"
  : >"$C2QUAY_CONFIG"
  export PACT_BROKER_USERNAME="pact"
  export PACT_BROKER_PASSWORD="test-pw"
  make_stub "check-compose-variables.sh" 0 ""
  unset ALT_ACK_FRESH_INSTALL
}

use_real_sweep() {
  unset LEFTOVER_SWEEP_SCRIPT
  export LEFTOVER_SWEEP_SCRIPT="$SWEEP"
}

c2quay_must_not_run() {
  if grep -q "c2quay" "$STUB_LOG"; then
    echo "  FAIL: c2quay / parent up must not run ($1)"
    cat "$STUB_LOG"
    return 1
  fi
}

make_docker_stub() {
  local ps_a_out="$1"
  local ps_out="$2"
  local path="$STUB_BIN/docker"
  {
    echo '#!/usr/bin/env bash'
    echo "echo \"[stub] docker \$*\" >> \"$STUB_LOG\""
    cat <<EOF
ps_a_out=$(printf '%q' "$ps_a_out")
ps_out=$(printf '%q' "$ps_out")
case "\$1" in
  ps)
    shift
    all=0
    for arg in "\$@"; do
      [ "\$arg" = "-a" ] && all=1
    done
    if [ "\$all" -eq 1 ]; then
      printf '%s' "\$ps_a_out"
      [ -n "\$ps_a_out" ] && printf '\n'
    else
      printf '%s' "\$ps_out"
      [ -n "\$ps_out" ] && printf '\n'
    fi
    exit 0
    ;;
  stop|rm)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
EOF
  } >"$path"
  chmod +x "$path"
  export DOCKER_BIN="$path"
}

tc_runs_pact_then_sweep_then_c2quay_on_success() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub "retire-alt-pki-agent-leftovers.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "cascade-pki-sidecars.sh" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_eq "$rc" "0" "happy path must exit 0" || { echo "$run_output"; return 1; }
  assert_order_in_log "pact-check.sh --broker" "retire-alt-pki-agent-leftovers.sh" "c2quay deploy" || return 1
  if grep -q "cascade-pki-sidecars" "$STUB_LOG"; then
    echo "  FAIL: Wave 4 retired cascade-pki-sidecars; deploy must not invoke it"
    cat "$STUB_LOG"
    return 1
  fi
  if grep -q "remove-orphans" "$STUB_LOG"; then
    echo "  FAIL: leftover cleanup must not use --remove-orphans"
    cat "$STUB_LOG"
    return 1
  fi
}

tc_aborts_when_pact_check_fails() {
  export_sut_env
  make_stub "pact-check.sh" 1 "contract regression"
  make_stub "retire-alt-pki-agent-leftovers.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "cascade-pki-sidecars.sh" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when pact-check fails" || return 1
  if grep -q "c2quay" "$STUB_LOG"; then
    echo "  FAIL: c2quay must not run when pact-check fails"
    return 1
  fi
  if grep -q "retire-alt-pki-agent-leftovers" "$STUB_LOG"; then
    echo "  FAIL: leftover sweep must not run when pact-check fails"
    return 1
  fi
  if grep -q "cascade-pki-sidecars" "$STUB_LOG"; then
    echo "  FAIL: cascade-pki-sidecars must not run when pact-check fails"
    return 1
  fi
}

tc_aborts_when_leftover_sweep_fails() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub "retire-alt-pki-agent-leftovers.sh" 1 "dual writers"
  make_stub c2quay 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when leftover sweep fails" || return 1
  if grep -q "c2quay" "$STUB_LOG"; then
    echo "  FAIL: c2quay / parent up must not run when leftover sweep fails"
    return 1
  fi
}

tc_aborts_when_c2quay_fails() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub "retire-alt-pki-agent-leftovers.sh" 0 ""
  make_stub c2quay 2 "can-i-deploy blocked"
  make_stub "cascade-pki-sidecars.sh" 0 ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when c2quay fails" || return 1
  if grep -q "cascade-pki-sidecars" "$STUB_LOG"; then
    echo "  FAIL: cascade-pki-sidecars must not run after c2quay failure"
    cat "$STUB_LOG"
    return 1
  fi
}

tc_defaults_env_to_production() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub "retire-alt-pki-agent-leftovers.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "cascade-pki-sidecars.sh" 0 ""

  "$SUT" >/dev/null 2>&1
  if ! stub_called_with c2quay deploy --env production --config "$C2QUAY_CONFIG"; then
    echo "  FAIL: c2quay must be called with --env production by default"
    cat "$STUB_LOG"
    return 1
  fi
}

tc_passes_custom_env_arg() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub "retire-alt-pki-agent-leftovers.sh" 0 ""
  make_stub c2quay 0 ""
  make_stub "cascade-pki-sidecars.sh" 0 ""

  "$SUT" staging >/dev/null 2>&1
  if ! stub_called_with c2quay deploy --env staging --config "$C2QUAY_CONFIG"; then
    echo "  FAIL: c2quay must receive staging env"
    cat "$STUB_LOG"
    return 1
  fi
}

tc_help_documents_fresh_install_ack() {
  export_sut_env
  make_stub "pact-check.sh" 0 ""
  make_stub "retire-alt-pki-agent-leftovers.sh" 0 ""
  make_stub c2quay 0 ""

  run_output=$("$SUT" --help 2>&1); rc=$?
  assert_eq "$rc" "0" "--help must exit 0" || { echo "$run_output"; return 1; }
  assert_contains "$run_output" "ALT_ACK_FRESH_INSTALL=1" "help must document the exact fresh-install ACK" || return 1
  c2quay_must_not_run "help" || return 1
  if grep -q "pact-check.sh" "$STUB_LOG"; then
    echo "  FAIL: --help must not run the deploy pipeline"
    cat "$STUB_LOG"
    return 1
  fi
}

tc_real_sweep_steady_zero_sidecar_is_noop_not_fresh_install() {
  export_sut_env
  use_real_sweep
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_docker_stub $'aaa111\talt-backend\nbbb222\tstep-ca' ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_eq "$rc" "0" "visible project anchors with zero pki-agent must be a steady no-op" || {
    echo "$run_output"
    cat "$STUB_LOG"
    return 1
  }
  assert_contains "$run_output" "alt-backend" "must list exact service labels for project=alt" || return 1
  assert_contains "$run_output" "step-ca" "must list every project=alt service label" || return 1
  assert_contains "$run_output" "pki-agent=0" "steady no-op must report matching pki-agent=0" || return 1
  if echo "$run_output" | grep -q "fresh-install"; then
    echo "  FAIL: matching pki-agent=0 with visible anchors is not a fresh install"
    echo "$run_output"
    return 1
  fi
  if grep -qE "docker stop|docker rm" "$STUB_LOG"; then
    echo "  FAIL: steady zero-sidecar must not stop/rm project anchors"
    cat "$STUB_LOG"
    return 1
  fi
  assert_order_in_log "pact-check.sh --broker" "c2quay deploy" || return 1
}

tc_real_sweep_stops_sidecars_not_parents() {
  export_sut_env
  use_real_sweep
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_docker_stub $'aaa111\talt-backend\nabc123\tpki-agent-alt-backend' ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_eq "$rc" "0" "leftover stop/rm then zero gate must allow parent up" || {
    echo "$run_output"
    cat "$STUB_LOG"
    return 1
  }
  assert_order_in_log "docker ps -a" "docker stop -- abc123" "docker rm -f -- abc123" "c2quay deploy" || return 1
  if grep -q "stop -- aaa111" "$STUB_LOG"; then
    echo "  FAIL: sweep must not stop project anchors"
    cat "$STUB_LOG"
    return 1
  fi
}

tc_empty_project_without_ack_refuses_parent_up() {
  export_sut_env
  use_real_sweep
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_docker_stub "" ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "empty project listing is not an automatic fresh install" || return 1
  c2quay_must_not_run "empty listing without ACK" || return 1
  assert_contains "$run_output" "ALT_ACK_FRESH_INSTALL=1" "must name the exact ACK to proceed on an empty host" || return 1
  if echo "$run_output" | grep -qi "context\|rootless"; then
    :
  else
    echo "  FAIL: empty listing must mention Docker context or rootless mismatch"
    echo "$run_output"
    return 1
  fi
}

tc_wrong_ack_value_refuses_empty_project() {
  export_sut_env
  use_real_sweep
  export ALT_ACK_FRESH_INSTALL=true
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_docker_stub "" ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "ALT_ACK_FRESH_INSTALL must be the exact value 1" || return 1
  c2quay_must_not_run "ACK=true" || return 1
}

tc_explicit_fresh_install_ack_allows_empty_host() {
  export_sut_env
  use_real_sweep
  export ALT_ACK_FRESH_INSTALL=1
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_docker_stub "" ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_eq "$rc" "0" "exact ACK allows a genuinely empty host" || {
    echo "$run_output"
    return 1
  }
  assert_contains "$run_output" "ALT_ACK_FRESH_INSTALL=1" "fresh ACK must be auditable in the log" || return 1
  if ! grep -q "com.docker.compose.project=alt" "$STUB_LOG"; then
    echo "  FAIL: sweep must filter exact compose project=alt label"
    cat "$STUB_LOG"
    return 1
  fi
  if grep -q "name=pki-agent" "$STUB_LOG"; then
    echo "  FAIL: sweep must not glob name=pki-agent (other projects)"
    cat "$STUB_LOG"
    return 1
  fi
  if grep -q "remove-orphans" "$STUB_LOG"; then
    echo "  FAIL: sweep must not use --remove-orphans"
    cat "$STUB_LOG"
    return 1
  fi
  assert_order_in_log "pact-check.sh --broker" "c2quay deploy" || return 1
}

tc_malformed_labels_fail_closed_even_with_ack() {
  export_sut_env
  use_real_sweep
  export ALT_ACK_FRESH_INSTALL=1
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_docker_stub $'deadbeef\t' ""

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "missing compose.service label must fail closed" || return 1
  c2quay_must_not_run "malformed labels" || return 1
  assert_contains "$run_output" "label" "malformed-label failure must mention the compose service label" || return 1
}

tc_real_sweep_refuses_parent_up_when_zero_gate_fails() {
  export_sut_env
  use_real_sweep
  make_stub "pact-check.sh" 0 ""
  make_stub c2quay 0 ""
  make_docker_stub $'aaa111\talt-backend\nabc123\tpki-agent-alt-backend' $'abc123\tpki-agent-alt-backend'

  run_output=$("$SUT" production 2>&1); rc=$?
  assert_ne "$rc" "0" "persisting leftover must refuse parent up" || return 1
  c2quay_must_not_run "zero-gate failure" || return 1
  assert_contains "$run_output" "dual writer" "zero-gate failure must name dual writers" || return 1
}

tc_deploy_source_has_no_remove_orphans() {
  if grep -E '^[^#]*--remove-orphans' "$SUT"; then
    echo "  FAIL: scripts/deploy.sh must not invoke --remove-orphans"
    grep -n -- '--remove-orphans' "$SUT"
    return 1
  fi
}

main() {
  echo "deploy tests (c2quay wrapper + leftover sweep)"
  run_case "chains pact-check → leftover sweep → c2quay and does not cascade" tc_runs_pact_then_sweep_then_c2quay_on_success
  run_case "aborts without leftover sweep or c2quay when pact-check fails" tc_aborts_when_pact_check_fails
  run_case "aborts without parent up when leftover sweep fails" tc_aborts_when_leftover_sweep_fails
  run_case "aborts without running cascade when c2quay fails" tc_aborts_when_c2quay_fails
  run_case "defaults env to production when no arg given" tc_defaults_env_to_production
  run_case "passes custom env arg through to c2quay" tc_passes_custom_env_arg
  run_case "--help documents ALT_ACK_FRESH_INSTALL=1 and does not deploy" tc_help_documents_fresh_install_ack
  run_case "visible anchors with zero pki-agent is a steady no-op, not a fresh install" tc_real_sweep_steady_zero_sidecar_is_noop_not_fresh_install
  run_case "leftover pki-agent is stop -- then rm -f -- without stopping parents" tc_real_sweep_stops_sidecars_not_parents
  run_case "empty project listing without ACK refuses parent up" tc_empty_project_without_ack_refuses_parent_up
  run_case "ACK=true is not accepted for an empty listing" tc_wrong_ack_value_refuses_empty_project
  run_case "ALT_ACK_FRESH_INSTALL=1 allows a genuinely empty host" tc_explicit_fresh_install_ack_allows_empty_host
  run_case "malformed compose.service labels fail closed even with ACK" tc_malformed_labels_fail_closed_even_with_ack
  run_case "zero-gate failure refuses parent up" tc_real_sweep_refuses_parent_up_when_zero_gate_fails
  run_case "deploy.sh source has no --remove-orphans" tc_deploy_source_has_no_remove_orphans
  summary
}

main "$@"
