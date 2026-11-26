#!/usr/bin/env bash
# Thin wrapper for Alt isolated live verification harness (VERIFY-04).
# Implementation lives in scripts/altharness (Go, `alt-harness verify-live`).
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
default_repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"
exec "${default_repo_root}/scripts/altharness/alt-harness" verify-live "$@"
