#!/usr/bin/env bash
# Validate that the custom Docker AppArmor profile for PostgreSQL is installed
# on the host, matches the repository source, compiles cleanly with
# apparmor_parser, and (unless --static-only is given) passes the live runtime
# container signal probe.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APPARMOR_SOURCE_PROFILE="${APPARMOR_SOURCE_PROFILE:-$REPO_ROOT/deploy/apparmor/alt-docker-postgres}"
APPARMOR_HOST_PROFILE="${APPARMOR_HOST_PROFILE:-/etc/apparmor.d/alt-docker-postgres}"
APPARMOR_PROFILE_NAME="${APPARMOR_PROFILE_NAME:-alt-docker-postgres}"
APPARMOR_PARSER_BIN="${APPARMOR_PARSER_BIN:-apparmor_parser}"
PROBE_SCRIPT="${PROBE_SCRIPT:-$REPO_ROOT/scripts/probe-apparmor-postgres.sh}"

MODE="probe"
for arg in "$@"; do
  case "$arg" in
    --static-only)
      MODE="static"
      ;;
    --probe)
      MODE="probe"
      ;;
    *)
      echo "Usage: $0 [--probe|--static-only]" >&2
      exit 2
      ;;
  esac
done

if [[ ! -f "$APPARMOR_SOURCE_PROFILE" ]] || [[ ! -r "$APPARMOR_SOURCE_PROFILE" ]]; then
  echo "FAIL: source AppArmor profile missing or unreadable: $APPARMOR_SOURCE_PROFILE" >&2
  exit 1
fi

if [[ ! -f "$APPARMOR_HOST_PROFILE" ]] || [[ ! -r "$APPARMOR_HOST_PROFILE" ]]; then
  echo "FAIL: host AppArmor profile missing or unreadable: $APPARMOR_HOST_PROFILE" >&2
  echo "      Provision it first with: sudo ./scripts/install-apparmor-profile.sh" >&2
  exit 1
fi

if ! cmp -s "$APPARMOR_SOURCE_PROFILE" "$APPARMOR_HOST_PROFILE"; then
  echo "FAIL: host AppArmor profile ($APPARMOR_HOST_PROFILE) differs from repo source ($APPARMOR_SOURCE_PROFILE)" >&2
  echo "      Re-run: sudo ./scripts/install-apparmor-profile.sh" >&2
  exit 1
fi

if ! "$APPARMOR_PARSER_BIN" --preprocess -K "$APPARMOR_HOST_PROFILE" >/dev/null; then
  echo "FAIL: apparmor_parser --preprocess failed on $APPARMOR_HOST_PROFILE" >&2
  exit 1
fi

# -Q skips kernel loading; -K also avoids the root-owned policy cache.
if ! "$APPARMOR_PARSER_BIN" -Q -K "$APPARMOR_HOST_PROFILE" >/dev/null; then
  echo "FAIL: apparmor_parser -Q compile validation failed on $APPARMOR_HOST_PROFILE" >&2
  exit 1
fi

if [[ "$MODE" == "static" ]]; then
  echo "PASS (static-only): $APPARMOR_HOST_PROFILE matches source and compiles cleanly; runtime container confinement UNVERIFIED."
  exit 0
fi

export APPARMOR_PROFILE_NAME
"$PROBE_SCRIPT"
