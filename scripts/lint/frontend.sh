#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: $0 <project-dir>" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
project_rel="${1#./}"
project_rel="${project_rel%/}"
project_dir="${repo_root}/${project_rel}"

if [ ! -d "$project_dir" ] || [ ! -f "${project_dir}/package.json" ]; then
  echo "error: ${project_rel} is not a frontend project directory (missing package.json)" >&2
  exit 2
fi

if [ ! -d "${project_dir}/node_modules" ]; then
  echo "error: ${project_rel}/node_modules is missing; run \`bun install --frozen-lockfile\` in ${project_rel}" >&2
  exit 1
fi

cd "$project_dir"

lint_rc=0
if bun run lint:check; then
  lint_status="pass"
else
  lint_rc=$?
  lint_status="fail"
fi

check_rc=0
if bun run check; then
  check_status="pass"
else
  check_rc=$?
  check_status="fail"
fi

echo "lint:check: ${lint_status} (exit ${lint_rc})"
echo "check: ${check_status} (exit ${check_rc})"

if [ "$lint_rc" -ne 0 ] || [ "$check_rc" -ne 0 ]; then
  exit 1
fi
