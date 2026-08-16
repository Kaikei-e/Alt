#!/usr/bin/env bash
# Cascades recreate of netns-sharing pki-agent sidecars when their parent
# container id has changed. Closes the gap that compose
# `depends_on.restart: true` leaves open when the deploy tool recreates a
# single parent service: the sidecar keeps pointing at the parent's old
# netns and silently loses its reverse-proxy listener.
#
# Invoked by scripts/deploy.sh right after the deploy tool returns.
# Idempotent — exits 0 when all sidecars already match their parents.
#
# Environment overrides (for tests):
#   DOCKER_BIN        — defaults to `docker`
#   COMPOSE_FILE      — defaults to compose/compose.yaml in the repo root
#   COMPOSE_PROJECT   — defaults to `alt`
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOCKER_BIN="${DOCKER_BIN:-docker}"
COMPOSE_FILE="${COMPOSE_FILE:-$REPO_ROOT/compose/compose.yaml}"
COMPOSE_PROJECT="${COMPOSE_PROJECT:-alt}"

# compose_service : sidecar_container_name : parent_container_name
# Add a row here when a new service uses `network_mode: service:X` for
# pki-agent. Container names are what compose creates: the service's own
# `container_name:` when it declares one, otherwise `alt-<service>-1`. A name
# that does not exist makes this script print "skip: ... not running" and exit
# 0, so scripts/compose-netns-cascade-audit.py checks both the coverage and
# the names against compose on every CI run.
NETNS_SIDECARS=(
  "pki-agent-acolyte-orchestrator:alt-pki-agent-acolyte-orchestrator-1:acolyte-orchestrator"
  "pki-agent-tag-generator:alt-pki-agent-tag-generator-1:alt-tag-generator-1"
  "pki-agent-recap-subworker:alt-pki-agent-recap-subworker-1:alt-recap-subworker-1"
  "pki-agent-news-creator:alt-pki-agent-news-creator-1:news-creator"
)

for entry in "${NETNS_SIDECARS[@]}"; do
  IFS=':' read -r svc sidecar parent <<<"$entry"

  parent_id="$("$DOCKER_BIN" inspect --format '{{.Id}}' "$parent" 2>/dev/null || true)"
  sidecar_netns="$("$DOCKER_BIN" inspect --format '{{.HostConfig.NetworkMode}}' "$sidecar" 2>/dev/null || true)"

  if [[ -z "$parent_id" ]]; then
    echo "skip: parent '$parent' not running"
    continue
  fi
  if [[ -z "$sidecar_netns" ]]; then
    echo "skip: sidecar '$sidecar' not running"
    continue
  fi

  expected="container:$parent_id"
  if [[ "$sidecar_netns" == "$expected" ]]; then
    echo "ok: $sidecar netns matches parent $parent"
    continue
  fi

  echo "netns mismatch for $sidecar (expected=$expected actual=$sidecar_netns) — cascading recreate"
  "$DOCKER_BIN" compose -f "$COMPOSE_FILE" -p "$COMPOSE_PROJECT" up -d --no-deps --force-recreate "$svc"
done
