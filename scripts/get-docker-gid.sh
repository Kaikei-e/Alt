#!/usr/bin/env bash
# Resolve the host docker group GID for compose group_add on forwarders.
# No magic fallback: a wrong GID silently breaks docker.sock access.
set -euo pipefail

gid="$(getent group docker | cut -d: -f3 || true)"
if [[ -z "${gid}" ]]; then
  echo "error: docker group GID not found (is the docker group present?)" >&2
  exit 1
fi
printf '%s\n' "$gid"
