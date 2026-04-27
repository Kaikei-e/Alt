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
# Calling `docker network prune --force --filter "until=2h"` here is
# safe by Docker's own contract: the prune command only removes
# networks "not referenced by any containers"
# (https://docs.docker.com/reference/cli/docker/network/prune/), so
# concurrent CI runs that are mid-flight stay untouched. The 2h floor
# protects long-running build / e2e jobs (which rarely exceed 2h)
# while still reclaiming pool from anything cancelled the previous
# day. Failures are intentionally swallowed — this is best-effort
# hygiene, not a precondition for the Hurl suite.
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
#   source "$ROOT/e2e/hurl/_lib/reclaim-network-pool.sh"
#   reclaim_network_pool

# Honour an opt-out env var for the rare debugging case where the
# operator wants to keep stale networks around (e.g. forensics on a
# previous failed run). Default is to reclaim.
: "${RECLAIM_NETWORK_POOL:=1}"

reclaim_network_pool() {
  if [[ "${RECLAIM_NETWORK_POOL}" != "1" ]]; then
    return 0
  fi
  # `until=2h` matches Go's ParseDuration syntax. The filter targets
  # network creation timestamp; any network actively attached to a
  # running container is protected by docker's own logic regardless
  # of age.
  docker network prune --force --filter "until=2h" >/dev/null 2>&1 || true
  return 0
}
