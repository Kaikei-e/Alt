#!/usr/bin/env bash
# Exact-label leftover pki-agent sweep for Compose project=alt.
#
# Dual-writer order (same as docs/runbooks/pki-agent-recovery.md and
# alt-deploy): list every project=alt container with its
# com.docker.compose.service label, stop -- then rm -f -- any whose
# service starts with pki-agent-, then docker ps must show zero
# pki-agent matches. Never uses `compose up --remove-orphans`.
#
# matching pki-agent=0 is NOT a fresh install:
#   - visible project anchors + zero sidecars → steady no-op
#   - no project=alt containers visible → abort unless
#     ALT_ACK_FRESH_INSTALL=1 (genuinely empty host only)
#   - missing/malformed service labels, wrong Docker context, and
#     rootless mismatch fail closed. ACK does not override those.
#
# Usage (from scripts/deploy.sh, before any PKI_ENROLLMENT=enabled parent up):
#   scripts/retire-alt-pki-agent-leftovers.sh
#   ALT_ACK_FRESH_INSTALL=1 scripts/retire-alt-pki-agent-leftovers.sh
set -euo pipefail

DOCKER_BIN="${DOCKER_BIN:-docker}"
COMPOSE_PROJECT="${COMPOSE_PROJECT:-alt}"
PROJECT_LABEL="com.docker.compose.project=${COMPOSE_PROJECT}"
# Compose service names: start with alnum, then alnum / . / _ / -
COMPOSE_SERVICE_RE='^[A-Za-z0-9][A-Za-z0-9._-]*$'

fail_malformed_label() {
  local cid="$1" svc="$2"
  echo "pki-agent leftover sweep: malformed or missing com.docker.compose.service label on project=${COMPOSE_PROJECT} container ${cid:-?} (service=${svc:-<empty>}). Fail closed — missing labels are not a fresh install." >&2
  exit 1
}

retire_alt_pki_agent_leftovers() {
  local -a ids=()
  local cid svc leftovers line
  local anchors=0
  local total=0

  while IFS= read -r line || [ -n "$line" ]; do
    [ -n "$line" ] || continue
    cid="${line%%$'\t'*}"
    if [ "$cid" = "$line" ]; then
      fail_malformed_label "$line" ""
    fi
    svc="${line#*$'\t'}"
    if [ -z "$cid" ] || [ -z "$svc" ] || ! [[ "$svc" =~ $COMPOSE_SERVICE_RE ]]; then
      fail_malformed_label "$cid" "$svc"
    fi
    total=$((total + 1))
    echo "pki-agent leftover sweep: project=${COMPOSE_PROJECT} id=${cid} service=${svc}"
    case "$svc" in
      pki-agent-*) ids+=("$cid") ;;
      *) anchors=$((anchors + 1)) ;;
    esac
  done < <("$DOCKER_BIN" ps -a \
    --filter "label=${PROJECT_LABEL}" \
    --format '{{.ID}}\t{{.Label "com.docker.compose.service"}}')

  if [ "$total" -eq 0 ]; then
    if [ "${ALT_ACK_FRESH_INSTALL:-}" = "1" ]; then
      echo "pki-agent leftover sweep: project=${COMPOSE_PROJECT} visible=0 pki-agent=0; ALT_ACK_FRESH_INSTALL=1 (genuinely empty host)"
    else
      echo "pki-agent leftover sweep: no com.docker.compose.project=${COMPOSE_PROJECT} containers are visible." >&2
      echo "matching pki-agent=0 is not a fresh install (wrong Docker context, rootless vs root daemon, or missing labels)." >&2
      echo "Refuse parent up. Re-run against the host Compose engine, or set ALT_ACK_FRESH_INSTALL=1 only if this host has never run project=${COMPOSE_PROJECT}." >&2
      exit 1
    fi
  elif [ "${#ids[@]}" -eq 0 ]; then
    echo "pki-agent leftover sweep: project=${COMPOSE_PROJECT} anchors=${anchors} pki-agent=0 (steady no-op)"
  else
    echo "pki-agent leftover sweep: project=${COMPOSE_PROJECT} anchors=${anchors} matching=${#ids[@]} — stop then rm"
    "$DOCKER_BIN" stop -- "${ids[@]}"
    "$DOCKER_BIN" rm -f -- "${ids[@]}"
  fi

  leftovers=$("$DOCKER_BIN" ps \
    --filter "label=${PROJECT_LABEL}" \
    --format '{{.ID}}\t{{.Label "com.docker.compose.service"}}' \
    | awk -F '\t' '$2 ~ /^pki-agent-/ { print }')
  if [ -n "$leftovers" ]; then
    echo "pki-agent leftovers still running in project ${COMPOSE_PROJECT} (dual writers). Refuse parent recreate." >&2
    printf '%s\n' "$leftovers" >&2
    exit 1
  fi
}

retire_alt_pki_agent_leftovers
