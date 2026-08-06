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
# Reads: ROOT, NODE_IMAGE.

install_suite_deps() {
  local install_root="$ROOT/e2e/playwright"

  echo "==> installing Playwright dependencies (default bridge — the staging network has no egress)" >&2
  docker run --rm \
    -v "$ROOT:$ROOT" \
    -w "$install_root" \
    -e PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
    -e npm_config_update_notifier=false \
    "$NODE_IMAGE" \
    npm ci --no-audit --no-fund
}
