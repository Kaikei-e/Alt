#!/usr/bin/env bash
# Fail-fast gate for unset Docker Compose interpolation variables.
#
# `docker compose config` resolves an unset `${VAR}` to an empty string and
# only mentions it as a warning on stderr. Nothing downstream reads that
# warning, so an incomplete .env reaches production as a service configured
# with "" — a wrong Meilisearch key ran for 24 hours that way (PM-2026-048).
# This script turns the warning into a non-zero exit before any container is
# touched.
#
# Only bare `${VAR}` references can fail here. `${VAR:-default}` is a
# deliberate default and `${VAR:?}` already aborts the render on its own.
#
# Env-file resolution note: compose/compose.yaml is a pure `include:`
# aggregator, and every included file resolves interpolation against the .env
# sitting in its own directory (compose/.env) before falling back to the
# parent model's environment. A top-level `--env-file` is therefore not
# authoritative for this stack, so the script deliberately does not offer
# one — to render against a different env file, stage it as BOTH ./.env and
# compose/.env, which is what .github/workflows/compose-audit.yaml does with
# .env.template.
#
# Usage:
#   ./scripts/check-compose-variables.sh
#
# Environment overrides (for tests / CI):
#   DOCKER_BIN        — defaults to `docker`
#   COMPOSE_FILE      — defaults to compose/compose.yaml in the repo root
#   COMPOSE_PROJECT   — defaults to `alt`
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOCKER_BIN="${DOCKER_BIN:-docker}"
COMPOSE_FILE="${COMPOSE_FILE:-$REPO_ROOT/compose/compose.yaml}"
COMPOSE_PROJECT="${COMPOSE_PROJECT:-alt}"

RENDER_ERR="$(mktemp)"
trap 'rm -f "$RENDER_ERR"' EXIT

render_rc=0
"$DOCKER_BIN" compose -f "$COMPOSE_FILE" -p "$COMPOSE_PROJECT" config -q \
  2>"$RENDER_ERR" || render_rc=$?

# Compose emits one warning per include model, so the same variable shows up
# many times. The backslashes come from the logrus text formatter quoting the
# message field when stderr is not a TTY; strip them so one pattern covers
# both the piped and the interactive format.
mapfile -t UNSET_VARS < <(
  # shellcheck disable=SC1003  # '\\' is a literal backslash for tr, not an escaped quote
  tr -d '\\' < "$RENDER_ERR" \
    | sed -n 's/.*The "\([A-Za-z_][A-Za-z0-9_]*\)" variable is not set.*/\1/p' \
    | sort -u
)

report_unset_vars() {
  echo "${#UNSET_VARS[@]} interpolation variable(s) are unset and render as an empty string:"
  for var in "${UNSET_VARS[@]}"; do
    echo "  - ${var}"
  done
  echo ""
  echo "Set each one in the env file compose reads (./.env, symlinked from"
  echo "compose/.env), or give the reference an explicit \${VAR:-default} in"
  echo "compose/. Do not deploy with these blank."
}

if [[ "$render_rc" -ne 0 ]]; then
  echo "FAIL: compose render failed (exit ${render_rc}) — the stack cannot be deployed."
  echo ""
  grep -v 'variable is not set' "$RENDER_ERR" || true
  if [[ "${#UNSET_VARS[@]}" -gt 0 ]]; then
    echo ""
    report_unset_vars
  fi
  exit 1
fi

# Informational ratchet: how many of the interpolation variables are declared
# required (`${VAR:?}`) rather than silently defaultable. Never fails the gate
# — it exists so the number moving the wrong way is visible in deploy logs.
VARIABLES_TABLE="$(
  "$DOCKER_BIN" compose -f "$COMPOSE_FILE" -p "$COMPOSE_PROJECT" config --variables \
    2>/dev/null || true
)"
if [[ -n "$VARIABLES_TABLE" ]]; then
  total_vars="$(awk 'NR > 1 && NF > 0' <<<"$VARIABLES_TABLE" | wc -l)"
  required_vars="$(awk 'NR > 1 && $2 == "true"' <<<"$VARIABLES_TABLE" | wc -l)"
  echo "interpolation variables: ${total_vars} total, ${required_vars} declared required (\${VAR:?})"
fi

if [[ "${#UNSET_VARS[@]}" -eq 0 ]]; then
  echo "PASS: every compose interpolation variable resolves."
  exit 0
fi

echo ""
echo -n "FAIL: "
report_unset_vars
exit 1
