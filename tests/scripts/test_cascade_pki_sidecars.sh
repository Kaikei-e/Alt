#!/usr/bin/env bash
# Tests for scripts/cascade-pki-sidecars.sh — post-deploy netns-sharing sidecar
# cascade. Covers the PM-2026-030 recurrence path that compose
# depends_on.restart: true does not close when c2quay force-recreates a single
# parent service.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/lib.sh"

SUT="$REPO_ROOT/scripts/cascade-pki-sidecars.sh"

export_sut_env() {
  export DOCKER_BIN="$STUB_BIN/docker"
  export COMPOSE_FILE="$SANDBOX/fake-compose.yaml"
  export COMPOSE_PROJECT="alt-test"
  : >"$COMPOSE_FILE"
}

# Stubs `docker` to respond to `inspect --format 'X' <name>` queries with the
# provided mapping and to record all `docker compose ...` invocations in the
# stub log. Mapping format: one line per container, "<name>|<id>|<netns>".
make_docker_stub() {
  local mapping="$1"
  make_conditional_stub docker '
    if [[ "$1" == "inspect" ]]; then
      # Parse: docker inspect --format "{{...}}" <name>
      format=""; target=""
      while [[ $# -gt 0 ]]; do
        case "$1" in
          --format) format="$2"; shift 2 ;;
          -f) format="$2"; shift 2 ;;
          inspect) shift ;;
          *) target="$1"; shift ;;
        esac
      done
      while IFS="|" read -r name id netns; do
        [[ "$name" == "$target" ]] || continue
        case "$format" in
          *Id*)         printf "%s\n" "$id"; exit 0 ;;
          *NetworkMode*) printf "%s\n" "$netns"; exit 0 ;;
        esac
      done <<<"'"$mapping"'"
      exit 1
    fi
    if [[ "$1" == "compose" ]]; then
      exit 0
    fi
    exit 0'
}

tc_noop_when_sidecar_netns_matches_parent() {
  export_sut_env
  # Both sidecars point at current parent container ids.
  make_docker_stub "acolyte-orchestrator|abc123|bridge
alt-pki-agent-acolyte-orchestrator-1|ignored|container:abc123
tag-generator|def456|bridge
alt-pki-agent-tag-generator-1|ignored|container:def456"

  run_output=$("$SUT" 2>&1); rc=$?
  assert_eq "$rc" "0" "cascade must exit 0 when netns matches" || { echo "$run_output"; return 1; }

  compose_calls=$(grep -c "^\[stub\] docker compose" "$STUB_LOG" 2>/dev/null || true)
  assert_eq "$compose_calls" "0" "must not invoke compose when netns matches" || { cat "$STUB_LOG"; return 1; }
}

tc_force_recreates_sidecar_with_stale_netns() {
  export_sut_env
  # acolyte sidecar is stuck on an old parent id.
  make_docker_stub "acolyte-orchestrator|new321|bridge
alt-pki-agent-acolyte-orchestrator-1|ignored|container:old999
tag-generator|def456|bridge
alt-pki-agent-tag-generator-1|ignored|container:def456"

  run_output=$("$SUT" 2>&1); rc=$?
  assert_eq "$rc" "0" "cascade must exit 0 after recreate" || { echo "$run_output"; return 1; }

  # Must call: docker compose -f <file> -p <proj> up -d --no-deps --force-recreate pki-agent-acolyte-orchestrator
  if ! grep -qF "compose -f" "$STUB_LOG" || \
     ! grep -qF "up -d --no-deps --force-recreate pki-agent-acolyte-orchestrator" "$STUB_LOG"; then
    echo "  FAIL: expected compose up --force-recreate for stale sidecar"
    cat "$STUB_LOG"
    return 1
  fi

  # tag-generator netns matched → must NOT be recreated.
  if grep -qF "pki-agent-tag-generator" "$STUB_LOG" | grep -q "force-recreate"; then
    echo "  FAIL: must not recreate healthy pki-agent-tag-generator"
    cat "$STUB_LOG"
    return 1
  fi
}

tc_skips_gracefully_when_parent_absent() {
  export_sut_env
  # Parent container missing → docker inspect returns empty id. Script must
  # not crash (set -e) and must not call compose.
  make_docker_stub "alt-pki-agent-acolyte-orchestrator-1|ignored|container:something
tag-generator|def456|bridge
alt-pki-agent-tag-generator-1|ignored|container:def456"

  run_output=$("$SUT" 2>&1); rc=$?
  assert_eq "$rc" "0" "must exit 0 when a parent is absent" || { echo "$run_output"; return 1; }
  assert_contains "$run_output" "skip" "must log skip reason for absent parent" || return 1
}

tc_handles_all_configured_netns_sharing_sidecars() {
  export_sut_env
  # Both sidecars stale → both must be recreated.
  make_docker_stub "acolyte-orchestrator|pA|bridge
alt-pki-agent-acolyte-orchestrator-1|ignored|container:old1
tag-generator|pB|bridge
alt-pki-agent-tag-generator-1|ignored|container:old2"

  "$SUT" >/dev/null 2>&1 || true

  grep -qF "force-recreate pki-agent-acolyte-orchestrator" "$STUB_LOG" || {
    echo "  FAIL: acolyte sidecar cascade missing"
    cat "$STUB_LOG"
    return 1
  }
  grep -qF "force-recreate pki-agent-tag-generator" "$STUB_LOG" || {
    echo "  FAIL: tag-generator sidecar cascade missing"
    cat "$STUB_LOG"
    return 1
  }
}

main() {
  echo "cascade-pki-sidecars.sh tests"
  run_case "noop when netns matches" tc_noop_when_sidecar_netns_matches_parent
  run_case "recreates sidecar when netns stale" tc_force_recreates_sidecar_with_stale_netns
  run_case "skips when parent absent" tc_skips_gracefully_when_parent_absent
  run_case "handles both configured sidecars" tc_handles_all_configured_netns_sharing_sidecars
  summary
}

main "$@"
