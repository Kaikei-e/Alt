#!/usr/bin/env bash
# Tests for scripts/docker-cleanup.sh — the disk reclamation timer that runs as
# root every 30 minutes. Covers the outage path it caused once: a compose
# service stopped, its container was pruned, its image became unreferenced, and
# an unfiltered `docker image prune -a -f` deleted the only local copy so
# `restart: unless-stopped` had nothing to start.
#
# Every docker call is stubbed. Nothing here touches a real daemon.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/lib.sh"

SUT="$REPO_ROOT/scripts/docker-cleanup.sh"

export_sut_env() {
    export LOG_FILE="$SANDBOX/cleanup.log"
    export DOCKER_CLEANUP_LOCKFILE="$SANDBOX/cleanup.lock"
    export DOCKER_CLEANUP_REPORT_STAMP="$SANDBOX/report.stamp"
    export DOCKER_ROOT_DIR="$SANDBOX/dockerroot"
    export COMPOSE_FILE="$SANDBOX/compose.yaml"
    export COMPOSE_PROJECT="alt-test"
    mkdir -p "$DOCKER_ROOT_DIR"
    : >"$COMPOSE_FILE"
}

# A stopped compose service whose image is referenced by nothing else.
# STOPPED_STATE controls the container's reported state, COMPOSE_OK controls
# whether `docker compose config --images` succeeds.
make_docker_stub() {
    local compose_ok="${1:-ok}"
    make_conditional_stub docker '
compose_ok="'"$compose_ok"'"
case "$1" in
  info) exit 0 ;;
  compose)
    # docker compose -f X -p Y config --images
    if [[ "$compose_ok" == "ok" ]]; then
      echo "ghcr.io/example/alt-knowledge-sovereign:main"
      exit 0
    fi
    exit 1
    ;;
  ps)
    # -a -q  -> ids; -a -q --filter status=... -> stopped ids; -as --format -> sizes
    if [[ "$*" == *"-as"* ]]; then
      echo "knowledge-sovereign|0B (virtual 24.1MB)"
      exit 0
    fi
    if [[ "$*" == *"status=exited"* ]]; then
      echo "cid-sovereign"
      exit 0
    fi
    if [[ "$*" == *"volume="* ]]; then
      exit 0
    fi
    echo "cid-sovereign"
    exit 0
    ;;
  container)
    # container inspect --format ... <id>
    for a in "$@"; do
      case "$a" in
        *Config.Labels*) echo "alt-test|/knowledge-sovereign|2020-01-01T00:00:00Z|2020-01-02T00:00:00Z"; exit 0 ;;
        *Config.Image*) printf "sha256:deadbeef\nghcr.io/example/alt-knowledge-sovereign:sha-abc123\n"; exit 0 ;;
      esac
    done
    exit 0
    ;;
  image)
    # image inspect --format {{.Id}} <ref>
    ref="${*: -1}"
    case "$ref" in
      sha256:deadbeef|ghcr.io/example/alt-knowledge-sovereign:sha-abc123) echo "sha256:deadbeef"; exit 0 ;;
      *) echo "No such image" >&2; exit 1 ;;
    esac
    ;;
  images)
    echo "sha256:deadbeef|2020-01-01 00:00:00 +0000 UTC|ghcr.io/example/alt-knowledge-sovereign:sha-abc123"
    echo "sha256:cafe0000|2020-01-01 00:00:00 +0000 UTC|ghcr.io/example/abandoned:old"
    exit 0
    ;;
  rmi) exit 0 ;;
  rm) exit 0 ;;
  volume)
    # volume ls -q --filter name=buildx...
    exit 0
    ;;
  builder|network) exit 0 ;;
  system) exit 0 ;;
  *) exit 0 ;;
esac'
}

tc_never_removes_image_of_stopped_compose_service() {
    export_sut_env
    make_docker_stub ok

    "$SUT" >/dev/null 2>&1

    if grep -qF "rmi ghcr.io/example/alt-knowledge-sovereign:sha-abc123" "$STUB_LOG"; then
        echo "  FAIL: removed the image of a stopped compose service"
        cat "$STUB_LOG"
        return 1
    fi
    # The abandoned image has no container and is old, so it must still go.
    grep -qF "rmi ghcr.io/example/abandoned:old" "$STUB_LOG" || {
        echo "  FAIL: expected the unreferenced old image to be removed"
        cat "$STUB_LOG"
        return 1
    }
}

tc_never_removes_compose_managed_container() {
    export_sut_env
    make_docker_stub ok

    "$SUT" >/dev/null 2>&1

    if grep -qE "^\[stub\] docker rm cid-sovereign" "$STUB_LOG"; then
        echo "  FAIL: removed a stopped container owned by a compose project"
        cat "$STUB_LOG"
        return 1
    fi
    grep -qF "container_kept=knowledge-sovereign" "$LOG_FILE" || {
        echo "  FAIL: expected a log line explaining the container was kept"
        cat "$LOG_FILE"
        return 1
    }
}

tc_never_calls_volume_prune() {
    export_sut_env
    make_docker_stub ok

    "$SUT" >/dev/null 2>&1

    if grep -qF "docker volume prune" "$STUB_LOG"; then
        echo "  FAIL: called volume prune — a stopped container's named volume is data"
        cat "$STUB_LOG"
        return 1
    fi
}

tc_never_calls_unfiltered_image_prune() {
    export_sut_env
    make_docker_stub ok

    "$SUT" >/dev/null 2>&1

    if grep -qF "docker image prune" "$STUB_LOG"; then
        echo "  FAIL: fell back to image prune instead of a targeted, protected sweep"
        cat "$STUB_LOG"
        return 1
    fi
}

tc_container_protection_survives_compose_unavailable() {
    export_sut_env
    make_docker_stub fail

    "$SUT" >/dev/null 2>&1

    grep -qF "compose_image_protection=unavailable" "$LOG_FILE" || {
        echo "  FAIL: expected a loud log line when compose enumeration fails"
        cat "$LOG_FILE"
        return 1
    }
    if grep -qF "rmi ghcr.io/example/alt-knowledge-sovereign:sha-abc123" "$STUB_LOG"; then
        echo "  FAIL: lost image protection when compose was unavailable"
        cat "$STUB_LOG"
        return 1
    fi
}

tc_dry_run_removes_nothing() {
    export_sut_env
    make_docker_stub ok

    "$SUT" --dry-run >/dev/null 2>&1

    if grep -qE "^\[stub\] docker (rmi|rm|volume rm) " "$STUB_LOG" ||
        grep -qF "prune" "$STUB_LOG"; then
        echo "  FAIL: --dry-run performed a removal"
        cat "$STUB_LOG"
        return 1
    fi
}

tc_reports_what_must_be_deleted_when_over_budget() {
    export_sut_env
    export MAX_DOCKER_SIZE_GB=0
    make_conditional_stub docker '
case "$1" in
  info) exit 0 ;;
  ps)
    if [[ "$*" == *"-as"* ]]; then echo "news-creator|2.38GB (virtual 3GB)"; exit 0; fi
    exit 0 ;;
  images) exit 0 ;;
  volume) exit 0 ;;
  system)
    if [[ "$*" == *"Volumes"* ]]; then
      echo "1|17.64GB|alt_meili_data"
      echo "0|67.3MB|unattached_db_data"
      exit 0
    fi
    if [[ "$*" == *"Images"* ]]; then
      echo "0|1.2GB|ghcr.io/example/stale:old"
      exit 0
    fi
    if [[ "$*" == *"BuildCache"* ]]; then exit 0; fi
    if [[ "$*" == *".Type"* ]]; then
      echo "Images|30GB|0B (0%)"
      echo "Containers|4GB|0B (0%)"
      echo "Local Volumes|47GB|77MB (0%)"
      echo "Build Cache|0B|0B"
      exit 0
    fi
    exit 0 ;;
  *) exit 0 ;;
esac'
    # 200 MB of fake docker root so du reports something over the 0GB budget.
    dd if=/dev/zero of="$DOCKER_ROOT_DIR/blob" bs=1M count=200 >/dev/null 2>&1

    "$SUT" --report >/dev/null 2>&1
    rc=$?
    assert_eq "$rc" "0" "--report must exit 0" || { cat "$LOG_FILE"; return 1; }

    for needle in "OVER BUDGET by" "alt_meili_data" "unattached_db_data" "SUMMARY: need"; do
        grep -qF "$needle" "$LOG_FILE" || {
            echo "  FAIL: shortfall report missing '$needle'"
            cat "$LOG_FILE"
            return 1
        }
    done
    if grep -qF "Manual intervention may be required" "$LOG_FILE"; then
        echo "  FAIL: still emits the non-actionable message"
        return 1
    fi
    unset MAX_DOCKER_SIZE_GB
}

main() {
    echo "docker-cleanup.sh tests"
    run_case "keeps the image of a stopped compose service" tc_never_removes_image_of_stopped_compose_service
    run_case "keeps compose-managed containers" tc_never_removes_compose_managed_container
    run_case "never calls volume prune" tc_never_calls_volume_prune
    run_case "never calls image prune" tc_never_calls_unfiltered_image_prune
    run_case "image protection survives compose unavailable" tc_container_protection_survives_compose_unavailable
    run_case "--dry-run removes nothing" tc_dry_run_removes_nothing
    run_case "reports what must be deleted when over budget" tc_reports_what_must_be_deleted_when_over_budget
    summary
}

main "$@"
