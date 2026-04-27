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
  # Scope the reclaim strictly to alt-staging-* networks so we never
  # touch unrelated networks on a shared self-hosted runner host
  # (security audit F-003 / OWASP A05). docker network prune does not
  # accept name-prefix filters, so we list + remove individually.
  # Docker's own logic protects networks an active container is
  # attached to (it returns an error and the `|| true` swallows it),
  # which means concurrent CI runs on the same host stay safe.
  docker network ls --format '{{.Name}}' --filter 'name=^alt-staging-' \
    | while IFS= read -r net; do
        [[ -z "$net" ]] && continue
        docker network rm "$net" >/dev/null 2>&1 || true
      done
  return 0
}
