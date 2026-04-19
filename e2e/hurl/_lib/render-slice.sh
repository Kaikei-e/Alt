#!/usr/bin/env bash
# Render a per-service slice of compose/compose.staging.yaml into an
# isolated project-name + network-name, stripping fields that would
# collide when two matrix jobs share a single Docker daemon.
#
# Usage
# -----
# Source this file from a run.sh, then call:
#
#   render_slice <profile>
#
# After return, the caller may read:
#
#   $SLICE      — absolute path to the rendered compose YAML
#   $SLICE_DIR  — its parent mktemp dir (cleaned up by caller)
#
# Env
# ---
#   STAGING_PROJECT_NAME  defaults to "alt-staging"
#                         CI sets "alt-staging-<svc>" so multiple matrix
#                         jobs on the same Docker daemon don't fight over
#                         network + container names.
#
# Why render at all?
# ------------------
# compose/compose.staging.yaml (the single source of truth) hardcodes:
#
#   name: alt-staging
#   networks.alt-staging.name: alt-staging
#   services.*.container_name: alt-staging-*
#   services.*.ports: [...]     # no-op under internal: true
#
# Two matrix jobs can't both create a network literally named
# "alt-staging" on the same daemon. Rather than split the monolith into
# per-service files (ADR-000765 future work), each run.sh renders its
# own slice at startup: `docker compose config --profile <svc>` gives us
# the resolved YAML, Python then removes the collision fields and
# rewrites the network name to match $STAGING_PROJECT_NAME.
#
# Security (audit F-005)
# ----------------------
# The rendered slice lives under `mktemp -d`, NOT inside the repo
# workspace. `docker compose config` bakes resolved environment values
# into its output; if a future compose.staging.yaml ever pulls a real
# secret via ${VAR:-...} that value would be visible in the slice. A
# workspace-local file would survive crashes and could be read by later
# jobs on a non-ephemeral runner. mktemp -d + trap cleanup in the
# caller keeps the blast radius to the current run.

set -euo pipefail

render_slice() {
  local profile="$1"
  : "${STAGING_PROJECT_NAME:=alt-staging}"
  export STAGING_PROJECT_NAME

  SLICE_DIR="$(mktemp -d)"
  SLICE="$SLICE_DIR/slice.yaml"
  export SLICE SLICE_DIR

  local raw="$SLICE_DIR/raw.yaml"
  docker compose -f compose/compose.staging.yaml \
    --profile "$profile" \
    -p "$STAGING_PROJECT_NAME" \
    config > "$raw"

  python3 - "$raw" "$SLICE" <<'PY'
import os
import sys
import yaml

src, dst = sys.argv[1], sys.argv[2]
with open(src) as f:
    d = yaml.safe_load(f)

# `-p` on compose can override top-level name; drop it so the slice is
# project-agnostic and the -p flag on subsequent compose calls wins.
d.pop("name", None)

# container_name collisions on shared daemon -> let compose auto-name.
# host port publishes are no-op under `internal: true` but stripping
# keeps the slice self-describing and dodges any future daemon that
# does bind them.
for svc in d.get("services", {}).values():
    svc.pop("container_name", None)
    svc.pop("ports", None)

# Each project owns its own network, named after the project so a
# sibling compose project on the same daemon gets a distinct resource.
name = os.environ["STAGING_PROJECT_NAME"]
nets = d.get("networks", {})
if "alt-staging" in nets:
    nets["alt-staging"]["name"] = name

with open(dst, "w") as f:
    yaml.safe_dump(d, f, default_flow_style=False, sort_keys=False)
PY

  rm -f "$raw"
}
