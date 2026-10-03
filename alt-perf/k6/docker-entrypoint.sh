#!/bin/sh
# docker-entrypoint.sh - Read API token from file if provided, then exec k6.
#
# Docker secrets are mounted at /run/secrets/<name> as files.
# K6 does not natively support the _FILE pattern, so we read them here.

set -e

if [ -n "$K6_API_TOKEN_FILE" ] && [ -f "$K6_API_TOKEN_FILE" ]; then
  export K6_API_TOKEN=$(cat "$K6_API_TOKEN_FILE")
fi

exec k6 "$@"
