#!/usr/bin/env bash
# Install the Compose build that production compose/*.yaml requires.
#
# The stack carries `pre_start:` lifecycle hooks, which Compose refuses as an
# unknown property before v5.4.0 ("additional properties 'pre_start' not
# allowed"). The operator host runs v5.5.0 (measured in
# scripts/compose-feature-gate.report.json); a hosted runner's stock plugin is
# older, so every `docker compose config` audit fails on files that render
# fine in production. Pinning the plugin keeps CI rendering what the operator
# renders instead of auditing a downgraded parser.
#
# Usage: ./scripts/ci-install-compose.sh
set -euo pipefail

COMPOSE_VERSION="${COMPOSE_VERSION:-v5.5.0}"
COMPOSE_SHA256="${COMPOSE_SHA256:-c57ab918abd5b05ca7e7d0f275875dd1330a695074f309dc9eab1b49efafcd4b}"
PLUGIN_DIR="${HOME}/.docker/cli-plugins"

arch="$(uname -m)"
if [ "$arch" != "x86_64" ]; then
  echo "ci-install-compose: pinned checksum covers x86_64 only, runner is $arch" >&2
  exit 1
fi

mkdir -p "$PLUGIN_DIR"
curl -fsSL -o "$PLUGIN_DIR/docker-compose" \
  "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-x86_64"
echo "${COMPOSE_SHA256}  ${PLUGIN_DIR}/docker-compose" | sha256sum --check --status
chmod +x "$PLUGIN_DIR/docker-compose"

docker compose version
