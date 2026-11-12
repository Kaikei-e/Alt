#!/usr/bin/env bash
# Install the Compose build that production compose/*.yaml requires.
#
# The stack carries `pre_start:` lifecycle hooks, which Compose refuses as an
# unknown property before v5.4.0 ("additional properties 'pre_start' not
# allowed"). The hooks of services that drop every capability carry their own
# `cap_add`, which the hook schema rejects on v5.5.0 and accepts on v5.6.1
# (measured in scripts/compose-feature-gate.report.json); from v5.6 a hook
# also inherits its service's `cap_drop`, so the same files cannot serve both.
# A hosted runner's stock plugin is older, so every `docker compose config`
# audit fails on files that render fine in production. Pinning the plugin
# keeps CI rendering what the operator renders instead of auditing a
# downgraded parser.
#
# Usage: ./scripts/ci-install-compose.sh
set -euo pipefail

COMPOSE_VERSION="${COMPOSE_VERSION:-v5.6.1}"
COMPOSE_SHA256="${COMPOSE_SHA256:-47737f4236e6d3ccea7291995203f8f018058783b4c3a9469119811a0b5db8c0}"
PLUGIN_DIR="${HOME}/.docker/cli-plugins"

arch="$(uname -m)"
if [ "$arch" != "x86_64" ]; then
  echo "ci-install-compose: pinned checksum covers x86_64 only, runner is $arch" >&2
  exit 1
fi

mkdir -p "$PLUGIN_DIR"
curl -fsSL \
  --retry 5 \
  --retry-delay 2 \
  --connect-timeout 10 \
  --max-time 120 \
  --retry-max-time 300 \
  -o "$PLUGIN_DIR/docker-compose" \
  "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-x86_64"
echo "${COMPOSE_SHA256}  ${PLUGIN_DIR}/docker-compose" | sha256sum --check --status
chmod +x "$PLUGIN_DIR/docker-compose"

docker compose version
