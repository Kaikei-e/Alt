#!/usr/bin/env bash
# e2e/hurl/_lib/compose-up-with-retry.sh
#
# Defense-in-depth for the libnetwork race that PM-2026-046 chased.
# Even after [[reclaim-network-pool]] was made concurrency-safe
# (2026-05-29), the Docker daemon itself sporadically returns
#
#     failed to set up container networking:
#       network NAME not found
#
# right after reporting `Network NAME Created`. docker/compose
# upstream tracks this as a "stale, won't fix" race
# (docker/compose#12862; docker/compose#9054 covers the related
# concurrent-attach case). On a 17-leg e2e matrix the residual rate is
# low but non-zero, so this helper wraps `docker compose up -d --wait`
# with a small bounded retry loop.
#
# On failure we tear the project down (`down -v --remove-orphans`)
# before retrying so the second attempt starts from a clean slate
# rather than fighting half-created containers.
#
# Usage
# -----
#   source "$ROOT/e2e/hurl/_lib/compose-up-with-retry.sh"
#   compose_up_with_retry meilisearch stub-backend search-indexer
#
# The caller must have already exported / set:
#   SLICE                  rendered compose slice path
#   STAGING_PROJECT_NAME   compose project name
#
# Env
# ---
#   COMPOSE_UP_MAX_ATTEMPTS   default 3
#   COMPOSE_UP_BACKOFF_SECS   default 5  (linear, between attempts)

: "${COMPOSE_UP_MAX_ATTEMPTS:=3}"
: "${COMPOSE_UP_BACKOFF_SECS:=5}"

compose_up_with_retry() {
  : "${SLICE:?SLICE must be set before compose_up_with_retry}"
  : "${STAGING_PROJECT_NAME:?STAGING_PROJECT_NAME must be set before compose_up_with_retry}"

  local attempt
  for attempt in $(seq 1 "$COMPOSE_UP_MAX_ATTEMPTS"); do
    if docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
         up -d --wait --wait-timeout 180 "$@"; then
      return 0
    fi

    if (( attempt < COMPOSE_UP_MAX_ATTEMPTS )); then
      echo "==> compose up attempt ${attempt}/${COMPOSE_UP_MAX_ATTEMPTS} failed; tearing project down before retry" >&2
      docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
        down -v --remove-orphans >/dev/null 2>&1 || true
      sleep "$COMPOSE_UP_BACKOFF_SECS"
    fi
  done

  echo "==> compose up exhausted ${COMPOSE_UP_MAX_ATTEMPTS} attempts; surfacing failure to caller" >&2
  return 1
}
