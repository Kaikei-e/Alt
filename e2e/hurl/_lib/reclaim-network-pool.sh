#!/usr/bin/env bash
# e2e/hurl/_lib/reclaim-network-pool.sh
#
# Best-effort reclaim of Docker's pre-defined address pool from networks
# left behind by prior runs. The default Docker daemon ships with ~31
# /16 sized pools (see `docs/reference/cli/dockerd/`). On a self-hosted
# CI runner that brings up a unique compose project per CI run (e.g.
# `alt-staging-knowledge-sovereign-<run_id>`), cancelled jobs and
# crashed runs leave the network behind — its `docker compose down`
# trap never fires when the runner kills the workflow process. The
# pool fills, and the next run greets us with:
#
#   Error response from daemon: all predefined address pools have been
#   fully subnetted
#
# Concurrency rule (PM-2026-046)
# ------------------------------
# The pre-2026-05-29 version of this helper called `docker network rm`
# on every network whose name matched `^alt-staging-`. The reasoning
# was "docker network rm refuses to touch a network attached to an
# active container". That contract is real but ONLY kicks in after
# attach. Between `Network Created` and the first `Container Starting`
# line, the network is empty and rm succeeds. With a 17-leg e2e matrix
# on a shared daemon, the reclaim step of one job would race the
# create-to-attach window of a sibling job and delete the sibling's
# freshly-created network, producing the
#
#     failed to set up container networking: network NAME not found
#
# symptom that PM-2026-046 chased across multiple consecutive runs.
# The current logic is therefore strictly conservative:
#
#   1. Step 1 only removes networks belonging to OUR project
#      (exact-match on $STAGING_PROJECT_NAME) so concurrent matrix
#      jobs are untouched.
#   2. Step 2 prunes untagged stragglers older than RECLAIM_MAX_AGE
#      whose name carries the com.docker.compose.project label —
#      defense-in-depth against runner crashes from older runs. The
#      `until=` filter guarantees an in-flight sibling create cannot
#      match.
#
# Why a runtime helper rather than a runner-host daemon.json change:
# the daemon.json `default-address-pools` enlargement is the proper
# foundational fix and is tracked separately on the deploy-runner
# side. Until that lands, this helper unblocks CI without touching
# host config and remains useful as defense-in-depth even after
# the foundational fix is in place.
#
# Usage
# -----
#   STAGING_PROJECT_NAME=alt-staging-search-indexer-12345
#   source "$ROOT/e2e/hurl/_lib/reclaim-network-pool.sh"
#   reclaim_network_pool
#
# Env
# ---
#   RECLAIM_NETWORK_POOL    1 (default) to enable, 0 to skip
#   RECLAIM_MAX_AGE         age threshold for the stale-network sweep
#                           (Docker filter syntax, default 30m). The
#                           sweep never deletes networks younger than
#                           this, so an in-flight sibling matrix job's
#                           network is always safe.

: "${RECLAIM_NETWORK_POOL:=1}"
: "${RECLAIM_MAX_AGE:=30m}"

reclaim_network_pool() {
  if [[ "${RECLAIM_NETWORK_POOL}" != "1" ]]; then
    return 0
  fi

  : "${STAGING_PROJECT_NAME:?STAGING_PROJECT_NAME must be set before reclaim_network_pool}"

  # Step 1 — remove networks from a previous run of THIS job. The
  # filter is an exact match (^NAME$) so a sibling job whose project
  # name shares the alt-staging-<svc>- prefix is never touched.
  docker network ls --format '{{.Name}}' \
      --filter "name=^${STAGING_PROJECT_NAME}$" \
    | while IFS= read -r net; do
        [[ -z "$net" ]] && continue
        docker network rm "$net" >/dev/null 2>&1 || true
      done

  # Step 2 — defense-in-depth: sweep stale alt-staging-* networks left
  # by crashed runners. `until=RECLAIM_MAX_AGE` only matches networks
  # whose creation timestamp is older than the threshold, so a
  # freshly-created sibling matrix-job network cannot be hit by this
  # branch. `label=com.docker.compose.project` keeps the prune scoped
  # to compose-created networks (ad-hoc `docker network create` results
  # without that label are left alone). Failures are swallowed
  # (best-effort hygiene).
  docker network prune --force \
      --filter "until=${RECLAIM_MAX_AGE}" \
      --filter "label=com.docker.compose.project" \
      >/dev/null 2>&1 || true

  return 0
}
