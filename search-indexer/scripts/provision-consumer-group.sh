#!/usr/bin/env bash
# Provision Redis Streams consumer groups for search-indexer.
#
# Consumer groups must be created by infrastructure / setup scripts
# (DECREE §8), not ad hoc inside the application Start() path.
# Prefer mq-hub CreateConsumerGroup RPC in production; this script is the
# local/compose bootstrap fallback.
#
# Usage:
#   REDIS_URL=redis://localhost:6379 STREAM_KEY=alt:events:articles \
#     GROUP_NAME=search-indexer-group ./provision-consumer-group.sh
set -euo pipefail

REDIS_URL="${REDIS_URL:-redis://localhost:6379}"
STREAM_KEY="${STREAM_KEY:-alt:events:articles}"
GROUP_NAME="${GROUP_NAME:-search-indexer-group}"
START_ID="${START_ID:-0}"

if ! command -v redis-cli >/dev/null 2>&1; then
  echo "redis-cli is required" >&2
  exit 1
fi

# Parse redis://host:port (simple form used in compose).
HOST_PORT="${REDIS_URL#redis://}"
HOST_PORT="${HOST_PORT%%/*}"

echo "Provisioning consumer group ${GROUP_NAME} on ${STREAM_KEY} via ${HOST_PORT}"
set +e
out="$(redis-cli -u "${REDIS_URL}" XGROUP CREATE "${STREAM_KEY}" "${GROUP_NAME}" "${START_ID}" MKSTREAM 2>&1)"
rc=$?
set -e
if [[ $rc -eq 0 ]]; then
  echo "created"
  exit 0
fi
if [[ "${out}" == BUSYGROUP* ]]; then
  echo "already exists"
  exit 0
fi
echo "failed: ${out}" >&2
exit "$rc"
