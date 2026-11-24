#!/usr/bin/env bash
# Thin wrapper (supports <service-dir> | --file <path> | --changed | --hook).
# The implementation lives in scripts/altharness (Go).
set -euo pipefail
root="$(cd -P "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd -P)"
exec "$root/scripts/altharness/alt-harness" layers "$@"
