#!/usr/bin/env bash
# Idempotent host installer for Alt's custom PostgreSQL Docker AppArmor profile.
#
# Sequence:
#   1. Compile/validate source profile BEFORE touching /etc/apparmor.d
#      (runs both `apparmor_parser --preprocess` and `apparmor_parser -Q`).
#   2. Install to /etc/apparmor.d/alt-docker-postgres with mode 0644 (root:root
#      when executed as root).
#   3. Always reload into the running kernel (`apparmor_parser -r -W`) even if
#      the file on disk was already identical, recovering from lost kernel state.
#   4. Verify via `scripts/check-apparmor-profile.sh`.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APPARMOR_SOURCE_PROFILE="${APPARMOR_SOURCE_PROFILE:-$REPO_ROOT/deploy/apparmor/alt-docker-postgres}"
APPARMOR_HOST_PROFILE="${APPARMOR_HOST_PROFILE:-/etc/apparmor.d/alt-docker-postgres}"
APPARMOR_PARSER_BIN="${APPARMOR_PARSER_BIN:-apparmor_parser}"
INSTALL_BIN="${INSTALL_BIN:-install}"
CHECK_SCRIPT="${CHECK_SCRIPT:-$REPO_ROOT/scripts/check-apparmor-profile.sh}"
SKIP_RUNTIME_PROBE="${SKIP_RUNTIME_PROBE:-0}"

for arg in "$@"; do
  case "$arg" in
    --no-probe)
      SKIP_RUNTIME_PROBE="1"
      ;;
    *)
      echo "Usage: $0 [--no-probe]" >&2
      exit 2
      ;;
  esac
done

if [[ ! -f "$APPARMOR_SOURCE_PROFILE" ]] || [[ ! -r "$APPARMOR_SOURCE_PROFILE" ]]; then
  echo "FAIL: source AppArmor profile missing or unreadable: $APPARMOR_SOURCE_PROFILE" >&2
  exit 1
fi

# Step 1: Validate syntax and full policy compilation BEFORE writing to /etc/apparmor.d.
if ! "$APPARMOR_PARSER_BIN" --preprocess -K "$APPARMOR_SOURCE_PROFILE" >/dev/null; then
  echo "FAIL: pre-install include/preprocess check failed for $APPARMOR_SOURCE_PROFILE" >&2
  exit 1
fi

if ! "$APPARMOR_PARSER_BIN" -Q -K "$APPARMOR_SOURCE_PROFILE" >/dev/null; then
  echo "FAIL: pre-install policy compilation (-Q) failed for $APPARMOR_SOURCE_PROFILE" >&2
  exit 1
fi

# Step 2: Install policy file with mode 0644 (and root:root ownership when running as root).
HOST_DIR="$(dirname "$APPARMOR_HOST_PROFILE")"
if [[ ! -d "$HOST_DIR" ]]; then
  echo "FAIL: target directory does not exist: $HOST_DIR" >&2
  exit 1
fi

if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
  "$INSTALL_BIN" -o root -g root -m 0644 "$APPARMOR_SOURCE_PROFILE" "$APPARMOR_HOST_PROFILE"
else
  "$INSTALL_BIN" -m 0644 "$APPARMOR_SOURCE_PROFILE" "$APPARMOR_HOST_PROFILE"
fi

# Step 3: Always reload policy into the kernel and write cache (-r -W), even if
# the file on disk was unchanged.
if ! "$APPARMOR_PARSER_BIN" -r -W "$APPARMOR_HOST_PROFILE"; then
  echo "FAIL: apparmor_parser -r -W failed to load $APPARMOR_HOST_PROFILE into kernel" >&2
  exit 1
fi

# Step 4: Verify installation.
if [[ "$SKIP_RUNTIME_PROBE" == "1" ]]; then
  "$CHECK_SCRIPT" --static-only
  echo "WARN: --no-probe specified; runtime container confinement and signal delivery are UNVERIFIED."
else
  "$CHECK_SCRIPT" --probe
fi
