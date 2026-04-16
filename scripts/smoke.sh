#!/usr/bin/env bash
# Post-rollout smoke probe for the four edge endpoints.
# Invoked by c2quay (deploy.smoke.command in c2quay.yml) and usable standalone.
#
# Exit 0 iff every endpoint responds within SMOKE_WAIT_SECONDS (default 10).
set -uo pipefail

SMOKE_WAIT_SECONDS="${SMOKE_WAIT_SECONDS:-10}"

URLS=(
  "http://localhost/health"
  "http://localhost:9000/v1/health"
  "http://localhost:9250/health"
  "http://localhost:7700/health"
)

failed=0
for url in "${URLS[@]}"; do
  if ! "${CURL_BIN:-curl}" -fsS --max-time "$SMOKE_WAIT_SECONDS" "$url" >/dev/null 2>&1; then
    echo "smoke FAIL: $url" >&2
    failed=$((failed + 1))
  fi
done

(( failed == 0 ))
