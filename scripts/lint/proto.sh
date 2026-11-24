#!/usr/bin/env bash
# Thin wrapper. The implementation lives in scripts/altharness (Go).
set -euo pipefail
readonly BUF_VERSION="1.48.0"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
exec "$root/scripts/altharness/alt-harness" lang proto "$@"
