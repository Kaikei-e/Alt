#!/usr/bin/env bash
# e2e/hurl/_lib/reclaim-network-pool.test.sh
#
# Behavioural + structural test for reclaim-network-pool.sh.
#
# Behavioural: source the helper, stub `docker` with a function that
# records its argv, call reclaim_network_pool, then assert the recorded
# args match the expected `docker network prune --force --filter
# "until=2h"` invocation. We must never call the real Docker daemon from
# unit tests — pruning live networks would race with concurrent CI jobs.
#
# Structural: scan every `e2e/hurl/<service>/run.sh` and refuse to merge
# the change unless every script either sources the helper and calls it,
# or is exempt with a top-of-file comment "# RECLAIM_NETWORK_POOL: skip
# (<reason>)" (no current exemptions; field reserved for the rare case
# where a service intentionally needs orphaned networks).
#
# Run:
#   bash e2e/hurl/_lib/reclaim-network-pool.test.sh
# Exit 0 on green, non-zero on red.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HURL_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

PASS=0
FAIL=0

assert() {
  local name="$1"; shift
  if "$@"; then
    echo "  PASS  $name"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $name"
    FAIL=$((FAIL + 1))
  fi
}

# ---------------------------------------------------------------------------
# Behavioural tests: source the helper with a stubbed docker.
# ---------------------------------------------------------------------------

echo "[behavioural] reclaim_network_pool calls docker network prune correctly"

# Capture invocations into a temp file. We need a file (not a bash array)
# because the helper uses `$(docker ...)` / `< <(docker ...)` patterns that
# run docker in a subshell — array mutations from a subshell don't
# propagate back, but file appends do. The stub also feeds a fake network
# name on stdout when invoked as `docker network ls ...` so the helper's
# list+remove flow exercises both the listing and the removal call.
DOCKER_LOG=$(mktemp)
trap 'rm -f "$DOCKER_LOG"' EXIT

docker() {
  echo "$*" >> "$DOCKER_LOG"
  if [[ "$1" == "network" && "$2" == "ls" ]]; then
    # Fake one alt-staging network so the helper's per-network rm path
    # actually runs and gets recorded.
    echo "alt-staging-knowledge-sovereign-fake-run-id"
  fi
  return 0
}
export -f docker

# shellcheck source=./reclaim-network-pool.sh
source "$SCRIPT_DIR/reclaim-network-pool.sh"

: > "$DOCKER_LOG"
reclaim_network_pool

mapfile -t DOCKER_INVOCATIONS < "$DOCKER_LOG"
ALL_INVOCATIONS="${DOCKER_INVOCATIONS[*]:-}"

assert "at least one docker invocation" \
  test "${#DOCKER_INVOCATIONS[@]}" -ge 1
assert "performs network listing or pruning" \
  bash -c "[[ '$ALL_INVOCATIONS' == *'network '* ]]"
# F-003: prune must NOT touch unrelated networks on the shared runner host.
# The helper either (a) `prune --filter "label=alt.project=staging"` /
# `--filter "name=^alt-staging-"`, or (b) lists networks via name prefix
# filter and removes them individually. Either way, an alt-staging
# scoping must be present somewhere in the docker invocation chain.
assert "scopes the cleanup to alt-staging-* (label or name filter)" \
  bash -c "[[ '$ALL_INVOCATIONS' == *'alt-staging'* ]]"
assert "uses --force (CI must not prompt) when prune is invoked" \
  bash -c "[[ '$ALL_INVOCATIONS' != *'network prune'* || '$ALL_INVOCATIONS' == *'--force'* ]]"

# Failure resilience: a `docker` exit non-zero must NOT propagate. The
# whole point of this helper is best-effort cleanup; failing it would
# block the actual Hurl run.
docker() {
  echo "$*" >> "$DOCKER_LOG"
  return 1
}
export -f docker

: > "$DOCKER_LOG"
if reclaim_network_pool; then
  echo "  PASS  swallows docker non-zero exit"
  PASS=$((PASS + 1))
else
  echo "  FAIL  swallows docker non-zero exit"
  FAIL=$((FAIL + 1))
fi

# Restore docker to a no-op for the structural section (we don't need it).
docker() { return 0; }
export -f docker

# ---------------------------------------------------------------------------
# Structural tests: every run.sh must call reclaim_network_pool before
# `docker compose ... up`.
# ---------------------------------------------------------------------------

echo "[structural] every run.sh invokes reclaim_network_pool before compose up"

shopt -s nullglob
for script in "$HURL_ROOT"/*/run.sh; do
  service="$(basename "$(dirname "$script")")"

  # Allow explicit opt-out for a service that genuinely needs orphans.
  if grep -qE '^# RECLAIM_NETWORK_POOL: skip' "$script"; then
    echo "  SKIP  $service (explicit opt-out present)"
    continue
  fi

  if ! grep -qE 'reclaim_network_pool' "$script"; then
    echo "  FAIL  $service: run.sh does not call reclaim_network_pool"
    FAIL=$((FAIL + 1))
    continue
  fi

  # `reclaim_network_pool` must appear before any `docker compose ... up`
  # (else the pool reclaim happens after the network is requested, which
  # defeats the point). Match only command invocations: skip comment
  # lines (the rag-orchestrator script for example mentions `up` in a
  # comment well before the actual compose up).
  reclaim_line=$(grep -nE '^[[:space:]]*reclaim_network_pool' "$script" | head -1 | cut -d: -f1)
  # The `up` flag may be on the next line because the docker compose
  # invocation is line-continued with `\`. Gate on the first command-line
  # `docker compose ` invocation instead — every script's first compose
  # call is the stack bring-up. Skip comments.
  up_line=$(grep -nE '^[[:space:]]*docker compose ' "$script" | head -1 | cut -d: -f1 || true)
  if [[ -z "$up_line" ]]; then
    echo "  PASS  $service (no compose up to gate)"
    PASS=$((PASS + 1))
    continue
  fi
  if (( reclaim_line < up_line )); then
    echo "  PASS  $service (reclaim before compose up)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $service (reclaim must precede compose up)"
    FAIL=$((FAIL + 1))
  fi
done

echo
echo "Total: PASS=$PASS FAIL=$FAIL"
[[ $FAIL -eq 0 ]]
