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

canary_audit_script="${script_dir}/canary_audit.py"
if [[ ! -f "${canary_audit_script}" ]]; then
  echo "Error: missing canary audit helper: ${canary_audit_script}" >&2
  exit 1
fi

work_tmp="$(mktemp -d "${TMPDIR:-/tmp}/alt-py-canary-XXXXXX")"
cleanup_work_tmp() {
  rm -rf "${work_tmp}"
}
trap cleanup_work_tmp EXIT

bandit_dep_state=""
bandit_dep_rc=0
set +e
bandit_dep_state="$(query_dev_dep bandit)"
bandit_dep_rc=$?
set -e
if [[ $bandit_dep_rc -ne 0 ]]; then
  echo "error: failed to inspect pyproject.toml for bandit (exit ${bandit_dep_rc})" >&2
  exit 1
fi
case "$bandit_dep_state" in
  declared)
    check_bandit="true"
    bandit_canary_suffix=", bandit"
    ;;
  not-declared)
    check_bandit="false"
    bandit_canary_suffix=""
    ;;
  *)
    echo "error: unexpected dev-dependency check output for bandit: ${bandit_dep_state}" >&2
    exit 1
    ;;
esac

pyrefly_dep_state=""
pyrefly_dep_rc=0
set +e
pyrefly_dep_state="$(query_dev_dep pyrefly)"
pyrefly_dep_rc=$?
set -e
if [[ $pyrefly_dep_rc -ne 0 ]]; then
  echo "error: failed to inspect pyproject.toml for pyrefly (exit ${pyrefly_dep_rc})" >&2
  exit 1
fi
case "$pyrefly_dep_state" in
  declared)
    check_pyrefly="true"
    pyrefly_canary_suffix=", pyrefly"
    ;;
  not-declared)
    check_pyrefly="false"
    pyrefly_canary_suffix=""
    ;;
  *)
    echo "error: unexpected dev-dependency check output for pyrefly: ${pyrefly_dep_state}" >&2
    exit 1
    ;;
esac

echo "==> canary audit (ruff${bandit_canary_suffix}${pyrefly_canary_suffix})"
if ! uv run --frozen --only-group dev python "${canary_audit_script}" python-canary \
  --project-dir "${target_dir}" \
  --work-dir "${work_tmp}/canary" \
  --check-bandit "${check_bandit}" \
  --check-pyrefly "${check_pyrefly}"; then
  exit 1
fi
rm -rf "${work_tmp}/canary"
steps+=("canary")
statuses+=("pass")

echo "==> source files check"
if ! uv run --frozen --only-group dev python "${canary_audit_script}" python-verify-files \
  --project-dir "${target_dir}"; then
  exit 1
fi
steps+=("source files")
statuses+=("pass")

python_versions_script="${script_dir}/check_python_versions.py"
if [[ -f "${python_versions_script}" ]]; then
  run_step "python versions" python3 "${python_versions_script}" \
    --repo-root "${repo_root}" \
    --project-dir "${project_dir}"
elif [[ -f "${repo_root}/scripts/lint/run.sh" ]]; then
  echo "Error: missing python version validator: ${python_versions_script}" >&2
  exit 1
fi

lint_policy="${LINT_POLICY:-fix-existing}"
if [[ "$lint_policy" == "block-new" ]]; then
  ruff_json="${work_tmp}/ruff.json"
  ruff_rc=0
  echo "==> ruff check (policy=block-new)"
  steps+=("ruff check")
  set +e
  uv run --frozen --only-group dev ruff check --output-format json . > "${ruff_json}"
  ruff_rc=$?
  set -e
  if [[ "$ruff_rc" -eq 0 ]]; then
    statuses+=("pass")
  elif [[ "$ruff_rc" -eq 1 ]]; then
    if python3 "${script_dir}/check_ratchet.py" check-fingerprints \
      --repo-root "${repo_root}" \
      --project-dir "${target_dir}" \
      --ruff-json "${ruff_json}" \
      --require-findings; then
      statuses+=("pass (block-new)")
    else
      fp_rc=$?
      statuses+=("fail (block-new exit ${fp_rc})")
      failed=1
    fi
  else
    cat "${ruff_json}" >&2 || true
    statuses+=("fail (ruff exit ${ruff_rc})")
    failed=1
  fi
else
  run_step "ruff check" uv run --frozen --only-group dev ruff check .
fi
run_step "ruff format" uv run --frozen --only-group dev ruff format --check .

if [[ "$bandit_dep_state" == "declared" ]]; then
  bandit_out="${work_tmp}/bandit.out"
  bandit_rc=0
  echo "==> bandit"
  steps+=("bandit")
  set +e
  uv run --frozen --only-group dev bandit -r . -ll -ii \
    -x ./.venv,./node_modules,./.mypy_cache,./.pytest_cache,./.ruff_cache,./build,./dist \
    2>&1 | tee "${bandit_out}"
  bandit_rc="${PIPESTATUS[0]}"
  set -e
  if ! uv run --frozen --only-group dev python "${canary_audit_script}" python-verify-bandit-scan \
    --project-dir "${target_dir}" \
    --bandit-out "${bandit_out}"; then
    exit 1
  fi
  if [[ "$bandit_rc" -eq 0 ]]; then
    statuses+=("pass")
  else
    statuses+=("fail (exit ${bandit_rc})")
    failed=1
  fi
else
  steps+=("bandit")
  statuses+=("not declared")
fi

run_optional_dev_step pyrefly "pyrefly" \
  uv run --frozen --all-extras --group dev pyrefly check

if [[ "$project_dir" == "scripts" ]]; then
  run_step "python3 compileall" python3 -m compileall -q -x '(\.venv|__pycache__|harness/corpus)' .
  shopt -s nullglob
  helper_tests=(tests/test-*-helpers.py tests/test-genre-report-on-error-stop.py tests/test-lint-canary.py tests/test-check-python-versions.py)
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
