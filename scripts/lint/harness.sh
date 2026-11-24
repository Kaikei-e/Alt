#!/usr/bin/env bash
# Thin wrapper. The implementation lives in scripts/altharness (Go).
set -euo pipefail
if [ "$#" -gt 1 ] || { [ "$#" -eq 1 ] && [ "$1" != "harness" ]; }; then
  echo "usage: $0 [harness]" >&2
  exit 2
fi
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
exec "$root/scripts/altharness/alt-harness" lint harness
