#!/usr/bin/env bash
# Reconcile the running observability stack with the configuration in this tree.
#
# Prometheus reads its config once — at start-up, or when told to reload. No
# stage of the deploy pipeline tells it to, so a config change lands on disk and
# stays inert until a human intervenes. This is that intervention, made
# idempotent and verifiable so it can run unattended from a timer.
#
# Modes:
#   (default)      reload Prometheus (and Alertmanager, if deployed), then
#                  verify the running config converged on the tree
#   --check        report drift and exit non-zero; change nothing
#   --if-changed   reload only when drift is present, or when the last
#                  successful reload is older than --max-reload-age. This is
#                  the timer entry point: the periodic no-op reload keeps
#                  `prometheus_config_last_reload_success_timestamp_seconds`
#                  advancing, which turns that metric into a liveness signal
#                  for this script rather than a record of the last human
#                  who remembered.
#
# Reload is not restart. Anything outside the config file — command-line flags,
# retention, a new compose service — still needs the container recreated.
# See docs/runbooks/observability-config-reload.md.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

PROMETHEUS_URL="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
ALERTMANAGER_URL="${ALERTMANAGER_URL:-http://127.0.0.1:9093}"
ALERTMANAGER_CONFIG="observability/alertmanager/alertmanager.yml"
# 26h by default: longer than the 24h heartbeat the timer produces, so a
# healthy host never trips it, and short enough that a dead reload path is
# visible the next day rather than the next incident.
MAX_RELOAD_AGE="${MAX_RELOAD_AGE:-93600}"
RELOAD_VERIFY_TIMEOUT="${RELOAD_VERIFY_TIMEOUT:-30}"

MODE="reload"
case "${1:-}" in
  --check) MODE="check" ;;
  --if-changed) MODE="if-changed" ;;
  "" | --reload) MODE="reload" ;;
  -h | --help)
    sed -n '2,25p' "${BASH_SOURCE[0]}"
    exit 0
    ;;
  *)
    echo "unknown argument: $1 (expected --check, --if-changed or no argument)" >&2
    exit 2
    ;;
esac

log() { printf '%s %s\n' "$(date -Is)" "$*"; }

drift_check() {
  python3 scripts/observability-drift-check.py \
    --repo-root "$REPO_ROOT" \
    --prometheus-url "$PROMETHEUS_URL" \
    --alertmanager-url "$ALERTMANAGER_URL" \
    "$@"
}

reload_timestamp() {
  # Read the gauge straight off /metrics rather than through the query API:
  # the query engine is one more thing that can be unhealthy while the
  # process is perfectly able to tell us when it last loaded its config.
  curl -fsS --max-time 5 "$PROMETHEUS_URL/metrics" \
    | awk '/^prometheus_config_last_reload_success_timestamp_seconds /{print $2}'
}

reload_succeeded() {
  curl -fsS --max-time 5 "$PROMETHEUS_URL/metrics" \
    | awk '/^prometheus_config_last_reload_successful /{print $2}'
}

reload_prometheus() {
  local before after status waited
  before="$(reload_timestamp || true)"
  log "prometheus: reload requested (previous successful load: ${before:-unknown})"

  if ! status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 15 \
      -X POST "$PROMETHEUS_URL/-/reload")"; then
    log "prometheus: POST /-/reload could not be delivered"
    return 1
  fi
  if [[ "$status" != "200" ]]; then
    log "prometheus: POST /-/reload returned HTTP $status"
    if [[ "$status" == "405" ]]; then
      log "prometheus: the lifecycle API is disabled. Either add"
      log "prometheus:   --web.enable-lifecycle to the prometheus command in compose/observability.yaml,"
      log "prometheus:   or send SIGHUP instead: docker kill -s HUP <prometheus container>"
    fi
    return 1
  fi

  waited=0
  while ((waited < RELOAD_VERIFY_TIMEOUT)); do
    after="$(reload_timestamp || true)"
    if [[ -n "$after" && "$after" != "$before" ]]; then
      log "prometheus: config reloaded (successful load at $after)"
      return 0
    fi
    if [[ "$(reload_succeeded || true)" == "0" ]]; then
      log "prometheus: reload REJECTED — the file did not parse. The process is"
      log "prometheus: still serving the previously loaded config, which is the"
      log "prometheus: safe outcome but not the intended one. Run"
      log "prometheus:   scripts/observability-validate.sh"
      log "prometheus: and check the container logs for the parse error."
      return 1
    fi
    sleep 1
    waited=$((waited + 1))
  done

  log "prometheus: reload accepted but the load timestamp did not advance within ${RELOAD_VERIFY_TIMEOUT}s"
  return 1
}

reload_alertmanager() {
  local status
  [[ -f "$ALERTMANAGER_CONFIG" ]] || return 0
  if ! curl -fsS --max-time 5 -o /dev/null "$ALERTMANAGER_URL/-/healthy" 2>/dev/null; then
    log "alertmanager: not reachable at $ALERTMANAGER_URL — skipping (not deployed yet?)"
    return 0
  fi
  if ! status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 15 \
      -X POST "$ALERTMANAGER_URL/-/reload")"; then
    log "alertmanager: POST /-/reload could not be delivered"
    return 1
  fi
  if [[ "$status" != "200" ]]; then
    log "alertmanager: POST /-/reload returned HTTP $status"
    log "alertmanager: fall back to SIGHUP: docker kill -s HUP <alertmanager container>"
    return 1
  fi
  log "alertmanager: config reloaded"
  return 0
}

grafana_advisory() {
  # Grafana polls the dashboards directory on the interval its provider
  # declares, but datasource and alerting provisioning are applied at start-up
  # only. A change there is inert until the container is recreated, and nothing
  # in Grafana reports that — hence an advisory rather than an action.
  command -v docker >/dev/null 2>&1 || return 0
  local container started started_epoch newest
  container="$(docker ps --filter 'name=grafana' --format '{{.Names}}' 2>/dev/null | head -1)"
  [[ -n "$container" ]] || return 0
  started="$(docker inspect -f '{{.State.StartedAt}}' "$container" 2>/dev/null)" || return 0
  started_epoch="$(date -d "$started" +%s 2>/dev/null)" || return 0
  newest="$(find observability/grafana/provisioning/datasources \
                 observability/grafana/provisioning/alerting \
                 -type f -newermt "@$started_epoch" 2>/dev/null | head -5)"
  if [[ -n "$newest" ]]; then
    log "grafana: datasource/alerting provisioning changed since the container started."
    log "grafana: these are start-up-only — recreate grafana to apply:"
    log "grafana:   docker compose -f compose/compose.yaml -p alt up -d --force-recreate grafana"
    while IFS= read -r f; do log "grafana:   changed: $f"; done <<<"$newest"
  fi
}

run_drift_check() {
  # `set -e` would abort on the non-zero exit that *is* this script's input,
  # so the status is captured explicitly rather than inherited.
  local rc=0
  drift_check "$@" || rc=$?
  return "$rc"
}

case "$MODE" in
  check)
    rc=0
    run_drift_check --max-reload-age "$MAX_RELOAD_AGE" || rc=$?
    grafana_advisory
    exit "$rc"
    ;;

  if-changed)
    rc=0
    run_drift_check --max-reload-age "$MAX_RELOAD_AGE" --quiet || rc=$?
    if [[ "$rc" -eq 0 ]]; then
      log "no drift and the reload heartbeat is current — nothing to do"
      grafana_advisory
      exit 0
    fi
    if [[ "$rc" -eq 2 ]]; then
      # Prometheus is not running. There is nothing to reconcile, and a
      # stopped Prometheus is not this unit's alarm to raise — treating it as
      # a failure here would make the reconcile timer flap through every
      # planned restart.
      log "prometheus not reachable — nothing to reconcile"
      exit 0
    fi
    if [[ "$rc" -ne 1 ]]; then
      log "drift check could not run (exit $rc)"
      exit "$rc"
    fi
    log "drift detected — reconciling"
    ;;

  reload) ;;
esac

# Validate before reloading when a validator is available. Prometheus rejects a
# malformed config and keeps the old one, so this is not a safety gate — it is
# how the operator finds out *why* nothing changed without reading logs.
if command -v promtool >/dev/null 2>&1 || [[ -n "${PROMTOOL:-}" ]]; then
  log "validating configuration before reload"
  if ! validation_output="$(scripts/observability-validate.sh 2>&1)"; then
    printf '%s\n' "$validation_output" >&2
    log "validation failed — not reloading"
    exit 1
  fi
else
  log "promtool unavailable — skipping pre-reload validation (Prometheus will reject a bad config on its own)"
fi

failed=0
reload_prometheus || failed=1
reload_alertmanager || failed=1

log "verifying the running config converged on the tree"
if ! run_drift_check --max-reload-age "$MAX_RELOAD_AGE"; then
  log "still drifting after reload — differences above need a restart, not a reload"
  failed=1
fi

grafana_advisory
exit "$failed"
