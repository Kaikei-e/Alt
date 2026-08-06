#!/usr/bin/env bash
# e2e/_lib/reclaim-network-pool.test.sh
#
# Behavioural + structural test for reclaim-network-pool.sh.
#
# Behavioural: source the helper, stub `docker` with a function that
# records its argv, call reclaim_network_pool, then assert the recorded
# args match the expected `docker network prune --force --filter
# "until=2h"` invocation. We must never call the real Docker daemon from
# unit tests — pruning live networks would race with concurrent CI jobs.
#
# Structural: scan every `e2e/playwright/<service>/run.sh` and refuse to
# merge the change unless the reclaim is reached before compose brings the
# stack up — either because the script goes through `suite_up` (which calls
# the helper itself, see e2e/playwright/_lib/suite.sh) or because it calls
# `reclaim_network_pool` directly. A script may opt out with a top-of-file
# comment "# RECLAIM_NETWORK_POOL: skip (<reason>)" for the rare case where
# a service intentionally needs orphaned networks.
#
# This scan globbed `e2e/hurl/*/run.sh` until the Playwright migration moved
# the suites and this helper. With `nullglob` the loop then matched nothing
# and the structural half reported green without checking anything — the
# exact silent-pass failure PM-2026-046 was written about. The zero-script
# guard below is why it cannot happen twice.
#
# Run:
#   bash e2e/_lib/reclaim-network-pool.test.sh
# Exit 0 on green, non-zero on red.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
E2E_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SUITE_ROOT="$E2E_ROOT/playwright"

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

# PM-2026-046 Fix 1 made the helper refuse to run without this — reclaiming by
# the `alt-staging-` prefix was what let one matrix leg delete a sibling's
# freshly created network. The test was never updated to set it, so from that
# commit until the Playwright migration the whole file exited non-zero on its
# first assertion and the structural half below never ran at all.
STAGING_PROJECT_NAME="alt-staging-selftest-$$"
export STAGING_PROJECT_NAME

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
# The regression PM-2026-046 was written about: a prefix match reclaimed a
# sibling matrix leg's network. Scoping must be to THIS project, exactly.
assert "scopes the name filter to this project exactly, not by prefix" \
  bash -c "[[ '$ALL_INVOCATIONS' == *'name=^${STAGING_PROJECT_NAME}\$'* ]]"
assert "uses --force (CI must not prompt) when prune is invoked" \
  bash -c "[[ '$ALL_INVOCATIONS' != *'network prune'* || '$ALL_INVOCATIONS' == *'--force'* ]]"

# Failure resilience: a `docker` exit non-zero must NOT propagate. The
# whole point of this helper is best-effort cleanup; failing it would
# block the actual E2E run.
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
scripts=("$SUITE_ROOT"/*/run.sh)
shopt -u nullglob

# A scan that matches nothing must be a failure, not a pass. See the header.
if [[ "${#scripts[@]}" -eq 0 ]]; then
  echo "  FAIL  no run.sh found under $SUITE_ROOT — this scan is checking nothing"
  FAIL=$((FAIL + 1))
fi

for script in ${scripts[@]+"${scripts[@]}"}; do
  service="$(basename "$(dirname "$script")")"

  # Allow explicit opt-out for a service that genuinely needs orphans.
  if grep -qE '^# RECLAIM_NETWORK_POOL: skip' "$script"; then
    echo "  SKIP  $service (explicit opt-out present)"
    continue
  fi

  # `suite_up` calls reclaim_network_pool before `compose up` on the caller's
  # behalf (e2e/playwright/_lib/suite.sh), so a script built on the shared
  # lifecycle satisfies this by construction — and asserting the ordering
  # inside suite.sh once is stronger than asserting it in twelve run.sh files.
  if grep -qE '^[[:space:]]*suite_up' "$script"; then
    echo "  PASS  $service (via suite_up)"
    PASS=$((PASS + 1))
    continue
  fi

  if ! grep -qE 'reclaim_network_pool' "$script"; then
    echo "  FAIL  $service: run.sh neither calls suite_up nor reclaim_network_pool"
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

# The shared lifecycle is now where the ordering actually lives, so assert it
# there too — otherwise every "PASS (via suite_up)" above rests on an unchecked
# assumption.
echo
echo "[structural] suite.sh reclaims the pool before bringing the stack up"
SUITE_SH="$SUITE_ROOT/_lib/suite.sh"
if [[ ! -f "$SUITE_SH" ]]; then
  echo "  FAIL  $SUITE_SH does not exist"
  FAIL=$((FAIL + 1))
else
  reclaim_line=$(grep -nE '^[[:space:]]*reclaim_network_pool' "$SUITE_SH" | head -1 | cut -d: -f1 || true)
  up_line=$(grep -nE '^[[:space:]]*if ! compose_up_with_retry' "$SUITE_SH" | head -1 | cut -d: -f1 || true)
  if [[ -z "$reclaim_line" ]]; then
    echo "  FAIL  suite.sh never calls reclaim_network_pool"
    FAIL=$((FAIL + 1))
  elif [[ -z "$up_line" ]]; then
    echo "  FAIL  suite.sh never calls compose_up_with_retry"
    FAIL=$((FAIL + 1))
  elif (( reclaim_line < up_line )); then
    echo "  PASS  suite.sh (reclaim at line $reclaim_line, compose up at line $up_line)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  suite.sh reclaims at line $reclaim_line, after compose up at line $up_line"
    FAIL=$((FAIL + 1))
  fi
fi

echo
echo "Total: PASS=$PASS FAIL=$FAIL"
[[ $FAIL -eq 0 ]]
