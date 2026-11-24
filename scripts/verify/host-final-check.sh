#!/usr/bin/env bash
# Host-side final verification wrapper combining worktree hygiene, targeted & repo lint,
# optional test command, LOOP-01 verify-handoff, and clean git status check.
# Thin wrapper: implementation lives in scripts/altharness (Go, `alt-harness host-final`).
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
default_repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"
exec "${default_repo_root}/scripts/altharness/alt-harness" host-final "$@"
