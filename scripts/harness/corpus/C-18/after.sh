#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOCKER_BIN="${DOCKER_BIN:-docker}"
COMPOSE_FILE="${COMPOSE_FILE:-$REPO_ROOT/compose/compose.yaml}"
COMPOSE_PROJECT="${COMPOSE_PROJECT:-alt}"
ENV_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file) ENV_FILE="$2"; shift 2 ;;
    *) echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$ENV_FILE" ]]; then
  echo "ERROR: --env-file is required" >&2
  exit 2
fi

RENDER_ERR="$(mktemp)"
trap 'rm -f "$RENDER_ERR"' EXIT

"$DOCKER_BIN" compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" -p "$COMPOSE_PROJECT" \
  config --no-env-resolution -q 2>"$RENDER_ERR"
if grep -q 'variable is not set' "$RENDER_ERR"; then
  echo "FAIL: unset compose interpolation variable detected" >&2
  exit 1
fi
