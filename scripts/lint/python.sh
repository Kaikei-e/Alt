#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <project-dir>" >&2
  exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"
project_dir="${1%/}"
target_dir="${repo_root}/${project_dir}"

if [[ ! -f "${target_dir}/pyproject.toml" ]]; then
  echo "Error: ${target_dir}/pyproject.toml not found" >&2
  exit 2
fi

cd -- "${target_dir}"

has_dev_dep() {
  local pkg="$1"
  uv run --frozen --only-group dev python -c '
import re, sys, tomllib

with open("pyproject.toml", "rb") as f:
    data = tomllib.load(f)

dev = data.get("dependency-groups", {}).get("dev", [])
target = re.sub(r"[-_.]+", "-", sys.argv[1]).lower()
for entry in dev:
    if isinstance(entry, str):
        name = re.split(r"[\s\[<>=!~;@]", entry.strip(), maxsplit=1)[0]
        if re.sub(r"[-_.]+", "-", name).lower() == target:
            sys.exit(0)
sys.exit(1)
' "$pkg"
}

failed=0
steps=()
statuses=()

run_step() {
  local name="$1"
  local rc=0
  shift
  echo "==> ${name}"
  steps+=("$name")
  if "$@"; then
    statuses+=("pass")
  else
    rc=$?
    statuses+=("fail (exit ${rc})")
    failed=1
  fi
}

run_step "ruff check" uv run --frozen --only-group dev ruff check .
run_step "ruff format" uv run --frozen --only-group dev ruff format --check .

bandit_rc=0
has_dev_dep bandit || bandit_rc=$?
if [[ $bandit_rc -eq 0 ]]; then
  run_step "bandit" uv run --frozen --only-group dev bandit -r . -ll -ii -x ./.venv,./node_modules,./.mypy_cache,./.pytest_cache,./.ruff_cache,./build,./dist
elif [[ $bandit_rc -eq 1 ]]; then
  steps+=("bandit")
  statuses+=("not declared")
else
  steps+=("bandit")
  statuses+=("fail (exit ${bandit_rc})")
  failed=1
fi

pyrefly_rc=0
has_dev_dep pyrefly || pyrefly_rc=$?
if [[ $pyrefly_rc -eq 0 ]]; then
  run_step "pyrefly" uv run --frozen --all-extras --group dev pyrefly check
elif [[ $pyrefly_rc -eq 1 ]]; then
  steps+=("pyrefly")
  statuses+=("not declared")
else
  steps+=("pyrefly")
  statuses+=("fail (exit ${pyrefly_rc})")
  failed=1
fi

echo ""
echo "Summary (${project_dir}):"
for i in "${!steps[@]}"; do
  echo "  ${steps[$i]}: ${statuses[$i]}"
done

exit "$failed"
