#!/usr/bin/env bash
# Provision Redis Streams consumer groups for search-indexer.
#
# Consumer groups must be created by infrastructure / setup scripts
# (DECREE §8), not ad hoc inside the application Start() path.
# Prefer mq-hub CreateConsumerGroup RPC in production; this script is the
# local/compose bootstrap fallback.
#
# redis-streams disables the default user, so this authenticates as the
# `streams` ACL user (override with REDIS_USER). The password is read from a
# file and handed to redis-cli through REDISCLI_AUTH, never argv or stdout.
#
# Usage:
#   REDIS_URL=redis://localhost:6379 STREAM_KEY=alt:events:articles \
#     GROUP_NAME=search-indexer-group \
#     ./provision-consumer-group.sh --password-file /path/to/redis_streams_password
#
#   REDIS_PASSWORD_FILE may stand in for --password-file.
set -euo pipefail

password_file="${REDIS_PASSWORD_FILE:-}"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --password-file)
      if [[ $# -lt 2 ]]; then
        echo "--password-file needs a path" >&2
        exit 2
      fi
      password_file="$2"
      shift 2
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

REDIS_URL="${REDIS_URL:-redis://localhost:6379}"
STREAM_KEY="${STREAM_KEY:-alt:events:articles}"
GROUP_NAME="${GROUP_NAME:-search-indexer-group}"
START_ID="${START_ID:-0}"

if ! command -v redis-cli >/dev/null 2>&1; then
  echo "redis-cli is required" >&2
  exit 1
fi

if [[ -z "${password_file}" ]]; then
  echo "REDIS_PASSWORD_FILE or --password-file is required: redis-streams accepts only the ${REDIS_USER:-streams} ACL user" >&2
  exit 1
fi
if ! REDISCLI_AUTH="$(cat -- "${password_file}" 2>/dev/null)" || [[ -z "${REDISCLI_AUTH}" ]]; then
  echo "password file is unreadable or empty: ${password_file}" >&2
  exit 1
fi
export REDISCLI_AUTH

# Parse redis://[user@]host[:port][/db]. redis-cli -u would read a bare
# userinfo as the password, so the user travels via --user instead.
authority="${REDIS_URL#redis://}"
authority="${authority%%/*}"
url_user=""
if [[ "${authority}" == *@* ]]; then
  url_user="${authority%@*}"
  authority="${authority##*@}"
fi
if [[ "${url_user}" == *:* ]]; then
  echo "REDIS_URL must not embed a password; use REDIS_PASSWORD_FILE or --password-file" >&2
  exit 1
fi
redis_user="${REDIS_USER:-${url_user:-streams}}"
redis_host="${authority%%:*}"
redis_port="6379"
if [[ "${authority}" == *:* ]]; then
  redis_port="${authority##*:}"
fi

echo "Provisioning consumer group ${GROUP_NAME} on ${STREAM_KEY} via ${redis_host}:${redis_port} as ${redis_user}"
set +e
out="$(redis-cli -h "${redis_host}" -p "${redis_port}" --user "${redis_user}" XGROUP CREATE "${STREAM_KEY}" "${GROUP_NAME}" "${START_ID}" MKSTREAM 2>&1)"
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
