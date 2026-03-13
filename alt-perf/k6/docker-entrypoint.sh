#!/bin/sh
# docker-entrypoint.sh - Read Docker secrets into environment variables, then exec k6.
#
# Docker secrets are mounted at /run/secrets/<name> as files.
# K6 does not natively support the _FILE pattern, so we read them here.

set -e

if [ -f /run/secrets/backend_token_secret ]; then
  export K6_BACKEND_TOKEN_SECRET=$(cat /run/secrets/backend_token_secret)
fi

exec k6 "$@"
