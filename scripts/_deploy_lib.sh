#!/usr/bin/env bash
# Helpers for scripts/deploy.sh. Sourced, not executed.
# Kept tiny and pure so tests can stub docker/curl/pact-broker-cli on PATH.

# Default layer ordering. One service per line, groups separated by blank lines.
# Layer 1 DB/storage is intentionally skipped (stateful — migrations go through Atlas).
DEFAULT_LAYERS=$'step-ca\n\nkratos\nauth-hub\n\nalt-backend\nsearch-indexer\nmq-hub\npre-processor\n\nnews-creator\ntag-generator\nrecap-worker\nrecap-subworker\nrecap-evaluator\nacolyte-orchestrator\nrag-orchestrator\ntts-speaker\n\nalt-butterfly-facade\nalt-frontend-sv\nnginx'

# Allow tests (and ops) to override the ordering with a pipe-separated list.
resolve_layers() {
  if [[ -n "${DEPLOY_LAYERS_OVERRIDE:-}" ]]; then
    tr '|' '\n' <<<"$DEPLOY_LAYERS_OVERRIDE"
  else
    printf '%s\n' "$DEFAULT_LAYERS"
  fi
}

# Polls `docker compose ps <svc>` until "healthy" appears or timeout expires.
# Treats "unhealthy" as transient — callers are responsible for interpreting timeout.
wait_until_healthy() {
  local svc="$1"
  local timeout="${HEALTHCHECK_TIMEOUT_SECONDS:-120}"
  local deadline=$(( $(date +%s) + timeout ))
  local output
  while (( $(date +%s) < deadline )); do
    output="$("${DOCKER_BIN:-docker}" compose -f "${COMPOSE_FILE}" ps "$svc" 2>/dev/null || true)"
    if echo "$output" | grep -qE '\bunhealthy\b|Exit '; then
      sleep 1
      continue
    fi
    if echo "$output" | grep -qE '\bhealthy\b'; then
      return 0
    fi
    # If the service has no healthcheck declared, docker reports state as
    # "running" (docker CLI) or "Up" (docker compose ps table). Accept both.
    if echo "$output" | grep -qE '\brunning\b|\bUp\b'; then
      return 0
    fi
    sleep 1
  done
  return 1
}

# Returns 0 when the service is present in the active compose file, non-zero
# otherwise. Some pacticipants (e.g. tts-speaker, deployed on a separate GPU
# host) live in their own compose file and must be skipped here without
# breaking the layered rollout.
service_in_compose() {
  local svc="$1"
  # Read the full service list into a variable before grep-ing so `set -o
  # pipefail` does not trip on SIGPIPE when grep -q exits early for services
  # near the top of the list.
  local services
  services="$("${DOCKER_BIN:-docker}" compose -f "${COMPOSE_FILE}" config --services 2>/dev/null)" || return 1
  grep -qx -- "$svc" <<<"$services"
}

# Deploy a single service by recreate, then wait for health.
deploy_single_service() {
  local svc="$1"
  echo "  [recreate] ${svc}"
  if [[ "${DRY_RUN:-0}" == "1" ]]; then
    echo "    (dry-run — skipping docker compose up)"
    return 0
  fi
  if ! service_in_compose "$svc"; then
    echo "    not defined on this host — skipping recreate (record-deployment still runs)"
    return 0
  fi
  "${DOCKER_BIN:-docker}" compose -f "${COMPOSE_FILE}" up -d --no-deps --force-recreate "$svc" || return 1
  if ! wait_until_healthy "$svc"; then
    echo "    healthcheck timeout for ${svc}" >&2
    return 1
  fi
  echo "    healthy"
}

# Global smoke tests — curl the edge endpoints after the full stack is rolled.
global_smoke() {
  if [[ "${DRY_RUN:-0}" == "1" ]]; then
    echo "  (dry-run — skipping global smoke)"
    return 0
  fi
  local failed=0
  for url in \
      "http://localhost/health" \
      "http://localhost:9000/v1/health" \
      "http://localhost:9250/health" \
      "http://localhost:7700/health"; do
    if ! "${CURL_BIN:-curl}" -fsS --max-time "${SMOKE_WAIT_SECONDS:-10}" "$url" >/dev/null 2>&1; then
      echo "  smoke FAIL: $url" >&2
      failed=$((failed+1))
    fi
  done
  (( failed == 0 ))
}

# Best-effort rollback: restore compose/ from the previous SHA and recreate.
rollback_to_previous() {
  local prev_sha_file="${DEPLOY_REPO_ROOT:-$REPO_ROOT}/.deploy-prev"
  echo "==> rolling back" >&2
  if [[ ! -r "$prev_sha_file" ]]; then
    echo "   no previous SHA recorded — nothing to restore" >&2
    return 1
  fi
  local prev_sha
  prev_sha="$(awk -F= '/^PREV_COMMIT=/{print $2}' "$prev_sha_file")"
  if [[ -z "$prev_sha" ]]; then
    echo "   malformed $prev_sha_file" >&2
    return 1
  fi
  (
    cd "${DEPLOY_REPO_ROOT:-$REPO_ROOT}"
    git checkout "$prev_sha" -- compose/ 2>/dev/null || true
  )
  if [[ "${DRY_RUN:-0}" == "1" ]]; then
    return 1
  fi
  "${DOCKER_BIN:-docker}" compose -f "${COMPOSE_FILE}" up -d --remove-orphans || true
  return 1
}

record_deployments() {
  local version="$1"
  local env="$2"
  local count=0
  local failed=0
  for svc in $DEPLOY_PACTICIPANTS; do
    if "${PACT_BROKER_BIN:-pact-broker-cli}" record-deployment \
        --pacticipant "$svc" \
        --version "$version" \
        --environment "$env" \
        --broker-base-url "${PACT_BROKER_BASE_URL}" \
        --broker-username "${PACT_BROKER_USERNAME}" \
        --broker-password "${PACT_BROKER_PASSWORD}" >/dev/null 2>&1; then
      count=$((count+1))
    else
      failed=$((failed+1))
      echo "  record-deployment: ${svc} failed" >&2
    fi
  done
  echo "recorded ${count} deployments against ${env}"
  if (( failed > 0 )); then
    echo "  ${failed} record-deployment call(s) failed — broker matrix is out of sync" >&2
    return 1
  fi
  return 0
}
