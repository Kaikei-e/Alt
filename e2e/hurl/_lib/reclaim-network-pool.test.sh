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

# Capture invocations into an array we can inspect.
declare -a DOCKER_INVOCATIONS=()
docker() {
  DOCKER_INVOCATIONS+=("$*")
  return 0
}
export -f docker

# shellcheck source=./reclaim-network-pool.sh
source "$SCRIPT_DIR/reclaim-network-pool.sh"

DOCKER_INVOCATIONS=()
reclaim_network_pool

assert "exactly one docker invocation" \
  test "${#DOCKER_INVOCATIONS[@]}" -eq 1
assert "subcommand is 'network prune'" \
  bash -c "[[ '${DOCKER_INVOCATIONS[0]:-}' == 'network prune'* ]]"
assert "uses --force (CI must not prompt)" \
  bash -c "[[ '${DOCKER_INVOCATIONS[0]:-}' == *'--force'* ]]"
assert "uses --filter until=... so active networks stay safe" \
  bash -c "[[ '${DOCKER_INVOCATIONS[0]:-}' == *'--filter'*'until='* ]]"

# Failure resilience: a `docker` exit non-zero must NOT propagate. The
# whole point of this helper is best-effort cleanup; failing it would
# block the actual Hurl run.
docker() {
  DOCKER_INVOCATIONS+=("$*")
  return 1
}
export -f docker

DOCKER_INVOCATIONS=()
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
