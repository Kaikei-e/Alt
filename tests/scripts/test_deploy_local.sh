#!/usr/bin/env bash
# Tests for deploy-system/deploy-local.sh's DOCKER_GROUP_ID guard (LOW
# finding): compose.yaml's logging include needs DOCKER_GROUP_ID to even
# parse (`${DOCKER_GROUP_ID:?...}` in compose/logging.yaml), which would
# otherwise surface as an opaque `docker compose ... build` failure instead
# of a clear, actionable message. The guard must fail fast, before any git
# or docker operation, when the var is unset.
#
# The rest of deploy-local.sh (git fetch/pull, docker compose build, c2quay
# deploy, smoke tests) needs network/docker and PROJECT_ROOT is derived from
# the script's own on-disk location rather than being test-overridable, so
# this suite only exercises the guard itself -- the one behavior added here
# -- not the full deploy flow.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/lib.sh"

SUT="$REPO_ROOT/deploy-system/deploy-local.sh"

tc_fails_fast_when_docker_group_id_unset() {
  make_stub c2quay 0 ""

  run_output=$(env -u DOCKER_GROUP_ID bash "$SUT" 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when DOCKER_GROUP_ID is unset" || { echo "$run_output"; return 1; }
  assert_contains "$run_output" "DOCKER_GROUP_ID" "error must name DOCKER_GROUP_ID" || return 1
  assert_contains "$run_output" "get-docker-gid.sh" "error must point at the remediation script" || return 1
}

tc_fails_fast_when_docker_group_id_empty() {
  make_stub c2quay 0 ""

  run_output=$(DOCKER_GROUP_ID="" bash "$SUT" 2>&1); rc=$?
  assert_ne "$rc" "0" "must exit non-zero when DOCKER_GROUP_ID is empty" || { echo "$run_output"; return 1; }
}

tc_guard_runs_before_git_operations() {
  make_stub c2quay 0 ""

  # A missing c2quay binary is checked before the guard, so remove the stub
  # from PATH for this case is not needed -- what matters is that with
  # DOCKER_GROUP_ID unset, the script never reaches (and logs) a git
  # fetch/pull step.
  run_output=$(env -u DOCKER_GROUP_ID bash "$SUT" 2>&1)
  if echo "$run_output" | grep -qi "pulling latest changes"; then
    echo "  FAIL: script proceeded to git pull despite DOCKER_GROUP_ID being unset"
    return 1
  fi
}

main() {
  echo "deploy-local.sh DOCKER_GROUP_ID guard tests"
  run_case "fails fast with remediation when DOCKER_GROUP_ID is unset" tc_fails_fast_when_docker_group_id_unset
  run_case "fails fast when DOCKER_GROUP_ID is set but empty" tc_fails_fast_when_docker_group_id_empty
  run_case "guard fires before any git operation" tc_guard_runs_before_git_operations
  summary
}

main "$@"
