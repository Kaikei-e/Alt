#!/usr/bin/env bash
# Thin wrapper. The implementation lives in scripts/altharness (Go).
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
exec "$root/scripts/altharness/alt-harness" lang observability "$@"
