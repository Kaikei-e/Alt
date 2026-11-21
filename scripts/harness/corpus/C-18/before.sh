#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOCKER_BIN="${DOCKER_BIN:-docker}"
COMPOSE_FILE="${COMPOSE_FILE:-$REPO_ROOT/compose/compose.yaml}"
COMPOSE_PROJECT="${COMPOSE_PROJECT:-alt}"

RENDER_ERR="$(mktemp)"
trap 'rm -f "$RENDER_ERR"' EXIT

"$DOCKER_BIN" compose -f "$COMPOSE_FILE" -p "$COMPOSE_PROJECT" config -q 2>"$RENDER_ERR"
if grep -q 'variable is not set' "$RENDER_ERR"; then
  echo "FAIL: unset compose interpolation variable detected" >&2
  exit 1
fi
