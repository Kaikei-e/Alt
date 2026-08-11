#!/usr/bin/env bash
# Install the fleet's Playwright dependencies into e2e/playwright/node_modules.
#
# Why a container, and why on the default bridge
# ----------------------------------------------
# The staging network is `internal: true`: containers on it have no route to
# the Internet. So the suite has to *run* inside that network to reach the
# service under test, but `npm ci` has to run *outside* it to reach the
# registry. This installs on the default bridge into the bind-mounted
# workspace; the test container then joins the staging network with
# node_modules already in place.
#
# Why one install root for twelve suites
# --------------------------------------
# `e2e/playwright/package.json` is the fleet's only manifest, and Node's module
# resolution walks up from `e2e/playwright/<service>/` to find it. One lockfile
# means twelve suites cannot drift onto different Playwright or zod versions —
# a drift that would otherwise show up as one suite failing on a matcher the
# others have — and a CI job that runs more than one suite installs once.
#
# Why not npm workspaces
# ----------------------
# `npm ci` from a workspace subdirectory installs into the *repository* root,
# not the subdirectory, and removes the existing node_modules before it does.
# With the repo bind-mounted at one path, parallel suites would each delete and
# rebuild the same tree underneath one another. The shared-root layout here
# gets the same deduplication with none of that.
#
# Why it retries
# --------------
# This is the one step that leaves the machine, so it fails the way networks
# fail. A registry read timeout here is indistinguishable, to the deploy gate,
# from the service under test being broken: the gate reads the e2e job as a
# whole, so one timeout withholds the entire production rollout. npm's own
# `fetch-retries` reuses the connection it just timed out on; a fresh container
# re-resolves DNS and dials again, which is what actually clears it.
#
# `npm ci` removes node_modules before it installs, so a half-finished attempt
# is not something the next one has to clean up.
#
# Reads: ROOT, NODE_IMAGE.
#
# Env
# ---
#   NPM_CI_MAX_ATTEMPTS   default 3
#   NPM_CI_BACKOFF_SECS   default 10  (linear, between attempts)

: "${NPM_CI_MAX_ATTEMPTS:=3}"
: "${NPM_CI_BACKOFF_SECS:=10}"

install_suite_deps() {
  local install_root="$ROOT/e2e/playwright"
  local attempt

  echo "==> installing Playwright dependencies (default bridge — the staging network has no egress)" >&2

  for attempt in $(seq 1 "$NPM_CI_MAX_ATTEMPTS"); do
    if docker run --rm \
         -v "$ROOT:$ROOT" \
         -w "$install_root" \
         -e PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
         -e npm_config_update_notifier=false \
         -e npm_config_fetch_retries=5 \
         -e npm_config_fetch_retry_maxtimeout=120000 \
         -e npm_config_fetch_timeout=600000 \
         "$NODE_IMAGE" \
         npm ci --no-audit --no-fund; then
      return 0
    fi

    if (( attempt < NPM_CI_MAX_ATTEMPTS )); then
      echo "==> npm ci attempt ${attempt}/${NPM_CI_MAX_ATTEMPTS} failed; retrying in ${NPM_CI_BACKOFF_SECS}s" >&2
      sleep "$NPM_CI_BACKOFF_SECS"
    fi
  done

  echo "==> npm ci exhausted ${NPM_CI_MAX_ATTEMPTS} attempts against the registry" >&2
  return 1
}
