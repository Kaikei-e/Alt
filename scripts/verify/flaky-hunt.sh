#!/usr/bin/env bash
# Flaky test hunter wrapper for Go and Python suites (VERIFY-05).
# Thin wrapper: implementation lives in scripts/altharness (Go, `alt-harness flaky-hunt`).
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
default_repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"
exec "${default_repo_root}/scripts/altharness/alt-harness" flaky-hunt "$@"
