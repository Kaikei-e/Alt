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

query_dev_dep() {
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
            print("declared")
            sys.exit(0)
print("not-declared")
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

run_optional_dev_step() {
  local pkg="$1"
  local label="$2"
  shift 2
  local dep_state="" dep_rc=0
  set +e
  dep_state="$(query_dev_dep "$pkg")"
  dep_rc=$?
  set -e
  if [[ $dep_rc -ne 0 ]]; then
    echo "error: failed to inspect pyproject.toml for ${pkg} (exit ${dep_rc})" >&2
    steps+=("$label")
    statuses+=("fail (dep check exit ${dep_rc})")
    failed=1
    return
  fi
  case "$dep_state" in
    declared)
      run_step "$label" "$@"
      ;;
    not-declared)
      steps+=("$label")
      statuses+=("not declared")
      ;;
    *)
      echo "error: unexpected dev-dependency check output for ${pkg}: ${dep_state}" >&2
      steps+=("$label")
      statuses+=("fail (unexpected output)")
      failed=1
      ;;
  esac
}

run_step "ruff check" uv run --frozen --only-group dev ruff check .
run_step "ruff format" uv run --frozen --only-group dev ruff format --check .

run_optional_dev_step bandit "bandit" \
  uv run --frozen --only-group dev bandit -r . -ll -ii -x ./.venv,./node_modules,./.mypy_cache,./.pytest_cache,./.ruff_cache,./build,./dist

run_optional_dev_step pyrefly "pyrefly" \
  uv run --frozen --all-extras --group dev pyrefly check

if [[ "$project_dir" == "scripts" ]]; then
  shopt -s nullglob
  helper_tests=(tests/test-*-helpers.py tests/test-genre-report-on-error-stop.py)
  shopt -u nullglob
  if [[ ${#helper_tests[@]} -eq 0 ]]; then
    echo "error: no helper tests matched under scripts/tests/" >&2
    steps+=("scripts helper tests")
    statuses+=("fail (none found)")
    failed=1
  else
    for test_file in "${helper_tests[@]}"; do
      run_step "$test_file" uv run --frozen --only-group dev python "$test_file"
    done
  fi
fi

echo ""
echo "Summary (${project_dir}):"
for i in "${!steps[@]}"; do
  echo "  ${steps[$i]}: ${statuses[$i]}"
done

exit "$failed"
